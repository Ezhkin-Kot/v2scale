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
		if _, resolvers, err := lc.QueryDNS(ctx, st.Self.DNSName, "A"); err == nil && len(resolvers) == 0 {
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
	raw, resolvers, err := lc.QueryDNS(ctx, name, qtype)
	if err != nil {
		return nil, err
	}
	if len(resolvers) > 0 {
		// The node chose to forward: its DNS configuration is not what we expect.
		return nil, fmt.Errorf("tailnet: resolver forwarded %q to upstream servers", name)
	}
	return raw, nil
}

func (n *Node) server() *tsnet.Server {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.srv
}

// DialContext dials inside the tailnet. Names from the config, from the tailnet's extra
// records and MagicDNS names are all resolved.
func (n *Node) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	n.mu.Lock()
	s, names := n.srv, n.names
	n.mu.Unlock()
	if s == nil {
		return nil, fmt.Errorf("tailnet: not started")
	}
	return names.wrap(s.Dial)(ctx, network, addr)
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
	n.mu.Lock()
	s := n.srv
	n.srv = nil
	n.mu.Unlock()
	if s == nil {
		return nil
	}
	return s.Close()
}
