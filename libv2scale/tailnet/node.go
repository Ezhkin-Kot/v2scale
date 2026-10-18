package tailnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

// Status is a snapshot of the node state for the UI.
type Status struct {
	// State is "stopped", "starting", "needs-login", "running" or a raw backend state.
	State string
	// AuthURL is the interactive login link while State is "needs-login".
	AuthURL string
	// IPs are the tailnet addresses once running.
	IPs []netip.Addr
}

// Node is an embedded Tailscale node.
type Node struct {
	cfg Config

	mu    sync.Mutex
	srv   *tsnet.Server
	names *nameTable
	ctrl  controlWatch
}

// NewNode validates cfg and returns a stopped node.
func NewNode(cfg Config) (*Node, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Node{cfg: cfg}, nil
}

// Start brings the node up in the background; use Status to follow progress or WaitUp
// to block until it is running.
func (n *Node) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.srv != nil {
		return fmt.Errorf("tailnet: already started")
	}
	if err := prepareEnv(n.cfg.StateDir); err != nil {
		return err
	}
	if n.cfg.ControlProxy != "" {
		u, err := url.Parse(n.cfg.ControlProxy)
		if err != nil {
			return err
		}
		controlProxy.Store(u)
	} else {
		controlProxy.Store(nil)
	}

	s := &tsnet.Server{
		Dir:        n.cfg.StateDir,
		Hostname:   n.cfg.Hostname,
		ControlURL: n.cfg.ControlURL,
		AuthKey:    n.cfg.AuthKey,
		Store:      n.cfg.Store,
		Ephemeral:  n.cfg.Ephemeral,
		Logf:       n.cfg.Logf,
	}
	if err := s.Start(); err != nil {
		return err
	}
	n.srv = s
	n.startControlWatch()
	n.names = newNameTable(n.cfg.Hosts, func(ctx context.Context) (*tailcfg.DNSConfig, error) {
		lc, err := s.LocalClient()
		if err != nil {
			return nil, err
		}
		return lc.DNSConfig(ctx)
	})
	return nil
}

// WaitUp blocks until the node is running and its DNS configuration is in place, or ctx
// ends. Until the DNS configuration has been applied the node's resolver would forward
// every query to upstream servers, so callers should not use DNS before WaitUp returns.
func (n *Node) WaitUp(ctx context.Context) error {
	s := n.server()
	if s == nil {
		return fmt.Errorf("tailnet: not started")
	}
	st, err := s.Up(ctx)
	if err != nil {
		return err
	}
	lc, err := s.LocalClient()
	if err != nil {
		return err
	}
	dc, err := lc.DNSConfig(ctx)
	if err != nil || len(dc.Domains) == 0 || st.Self == nil || st.Self.DNSName == "" {
		return nil // MagicDNS is off for this tailnet, nothing to wait for
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if raw, _, err := lc.QueryDNS(ctx, st.Self.DNSName, "A"); err == nil && isLocalAnswer(raw) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}

// ErrNotTailnetName is returned by QueryDNS for names outside the tailnet.
var ErrNotTailnetName = errors.New("tailnet: not a tailnet name")

// QueryDNS asks the node's resolver (MagicDNS, split DNS and the control server's extra
// records) and returns the raw DNS response. Names that do not belong to the tailnet are
// refused without being sent anywhere, because the node's resolver would forward them to
// upstream servers.
func (n *Node) QueryDNS(ctx context.Context, name, qtype string) ([]byte, error) {
	n.mu.Lock()
	s, names := n.srv, n.names
	n.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("tailnet: not started")
	}
	if !names.isTailnetName(ctx, name) {
		return nil, ErrNotTailnetName
	}
	lc, err := s.LocalClient()
	if err != nil {
		return nil, err
	}
	raw, _, err := lc.QueryDNS(ctx, name, qtype)
	if err != nil || !isLocalAnswer(raw) {
		// The node's resolver is not ready (after a network change its DNS config is gone until
		// control reconnects, and it would forward the name upstream). Known extra records and
		// static hosts can still be answered from what we last learned.
		if a, ok := names.cached(name); ok {
			if resp, ok := synthesizeAnswer(name, qtype, a); ok {
				return resp, nil
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if !isLocalAnswer(raw) {
		// The resolver forwarded the query (its DNS configuration is not applied yet, or the
		// name is not ours after all). The returned resolvers list cannot tell us this: it
		// names the upstreams that would be used, and is non-empty on any tailnet with
		// global nameservers even when the answer is local.
		return nil, fmt.Errorf("tailnet: %q was not answered by the node's own zone", name)
	}
	return raw, nil
}

// synthesizeAnswer builds an authoritative A or AAAA response for a known address. A name of
// the other family gets an empty (NODATA) answer, the name exists. Other types are not handled.
func synthesizeAnswer(name, qtype string, addr netip.Addr) ([]byte, bool) {
	if qtype != "A" && qtype != "AAAA" {
		return nil, false
	}
	n, err := dnsmessage.NewName(strings.TrimSuffix(name, ".") + ".")
	if err != nil {
		return nil, false
	}
	t := dnsmessage.TypeA
	if qtype == "AAAA" {
		t = dnsmessage.TypeAAAA
	}
	q := dnsmessage.Question{Name: n, Type: t, Class: dnsmessage.ClassINET}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{Response: true, Authoritative: true, RecursionAvailable: true})
	if b.StartQuestions() != nil || b.Question(q) != nil || b.StartAnswers() != nil {
		return nil, false
	}
	hdr := dnsmessage.ResourceHeader{Name: n, Class: dnsmessage.ClassINET, TTL: 30}
	addr = addr.Unmap()
	switch {
	case t == dnsmessage.TypeA && addr.Is4():
		b.AResource(hdr, dnsmessage.AResource{A: addr.As4()})
	case t == dnsmessage.TypeAAAA && addr.Is6():
		b.AAAAResource(hdr, dnsmessage.AAAAResource{AAAA: addr.As16()})
	}
	out, err := b.Finish()
	return out, err == nil
}

// isLocalAnswer reports whether a DNS response comes from the node's own zone: Tailscale
// marks those answers authoritative (AA), forwarded answers from upstream servers are not.
func isLocalAnswer(raw []byte) bool {
	return len(raw) >= 4 && raw[2]&0x04 != 0
}

func (n *Node) server() *tsnet.Server {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.srv
}

// DialContext dials inside the tailnet. Names from the config, from the tailnet's extra
// records and MagicDNS names are resolved here, by the node, and never by anything else: tsnet's
// own dialer falls back to the process's system resolver for a name it does not know, which would
// send that name to the local network's DNS server in the clear (this app is excluded from the VPN).
// A name that is not part of the tailnet is refused instead.
func (n *Node) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	n.mu.Lock()
	s, names := n.srv, n.names
	n.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("tailnet: not started")
	}
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if _, perr := netip.ParseAddr(host); perr != nil {
			ip, err := n.resolveName(ctx, names, host)
			if err != nil {
				return nil, err
			}
			addr = net.JoinHostPort(ip.String(), port)
		}
	}
	return s.Dial(ctx, network, addr)
}

// resolveName resolves a tailnet name through the node's own zone, or refuses it.
func (n *Node) resolveName(ctx context.Context, names *nameTable, host string) (netip.Addr, error) {
	if ip, ok := names.lookup(ctx, host); ok {
		return ip, nil
	}
	if !names.isTailnetName(ctx, host) {
		return netip.Addr{}, fmt.Errorf("%w: %q", ErrNotTailnetName, host)
	}
	for _, qtype := range []string{"A", "AAAA"} {
		raw, err := n.QueryDNS(ctx, host, qtype)
		if err != nil {
			return netip.Addr{}, err
		}
		if ip, ok := firstAddress(raw); ok {
			return ip, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("tailnet: no address for %q", host)
}

// firstAddress returns the first A or AAAA record of a DNS response.
func firstAddress(raw []byte) (netip.Addr, bool) {
	var p dnsmessage.Parser
	if _, err := p.Start(raw); err != nil {
		return netip.Addr{}, false
	}
	if p.SkipAllQuestions() != nil {
		return netip.Addr{}, false
	}
	for {
		h, err := p.AnswerHeader()
		if err != nil {
			return netip.Addr{}, false
		}
		switch h.Type {
		case dnsmessage.TypeA:
			if r, err := p.AResource(); err == nil {
				return netip.AddrFrom4(r.A), true
			}
		case dnsmessage.TypeAAAA:
			if r, err := p.AAAAResource(); err == nil {
				return netip.AddrFrom16(r.AAAA), true
			}
		default:
			if p.SkipAnswer() != nil {
				return netip.Addr{}, false
			}
		}
	}
}

// Status reports the current node state.
func (n *Node) Status(ctx context.Context) Status {
	s := n.server()
	if s == nil {
		return Status{State: "stopped"}
	}
	lc, err := s.LocalClient()
	if err != nil {
		return Status{State: "starting"}
	}
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return Status{State: "starting"}
	}
	switch {
	case st.AuthURL != "":
		return Status{State: "needs-login", AuthURL: st.AuthURL}
	case st.BackendState == "Running" && len(st.TailscaleIPs) > 0:
		return Status{State: "running", IPs: st.TailscaleIPs}
	default:
		return Status{State: strings.ToLower(st.BackendState)}
	}
}

// Close stops the node. A closed Node cannot be started again; create a new one.
func (n *Node) Close() error {
	n.ctrl.stop()
	n.mu.Lock()
	s := n.srv
	n.srv = nil
	n.mu.Unlock()
	if s == nil {
		return nil
	}
	return s.Close()
}
