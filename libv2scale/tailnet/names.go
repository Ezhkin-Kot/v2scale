package tailnet

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"tailscale.com/tailcfg"
)

const (
	namesRefresh    = 5 * time.Minute
	namesMissWindow = 5 * time.Second
)

// nameTable knows which names belong to the tailnet and resolves the ones tsnet cannot.
//
// Headscale publishes extra_records as A records with an explicit type, which tsnet's
// dialer ignores (it only honours records with an empty type), so names such as
// svc.example.net fail with "no such host". The table consults the static hosts from the
// config, then the tailnet's DNS.ExtraRecords fetched from the node. Everything else,
// including ordinary MagicDNS peer names, goes to tsnet unchanged.
//
// It also answers "is this a tailnet name?" (a name under one of the tailnet's DNS
// domains, an extra record, or a static host), which gates what the DNS bridge may ask
// the node's resolver: that resolver forwards unknown names to upstream servers.
type nameTable struct {
	static map[string]netip.Addr
	fetch  func(ctx context.Context) (*tailcfg.DNSConfig, error)

	mu      sync.Mutex
	extra   map[string]netip.Addr
	domains []string
	fetched time.Time
}

func newNameTable(hosts map[string]string, fetch func(context.Context) (*tailcfg.DNSConfig, error)) *nameTable {
	t := &nameTable{static: map[string]netip.Addr{}, fetch: fetch}
	for h, ip := range hosts {
		if a, err := netip.ParseAddr(ip); err == nil {
			t.static[normalizeName(h)] = a
		}
	}
	return t
}

func normalizeName(h string) string { return strings.ToLower(strings.TrimSuffix(h, ".")) }

// refreshLocked reloads the DNS config when it is stale, or when force is set and the
// last reload was not just now. t.mu must be held.
func (t *nameTable) refreshLocked(ctx context.Context, force bool) {
	if t.fetch == nil {
		return
	}
	age := time.Since(t.fetched)
	if age <= namesRefresh && !(force && age > namesMissWindow) {
		return
	}
	dc, err := t.fetch(ctx)
	if err != nil || dc == nil {
		return
	}
	m := map[string]netip.Addr{}
	for _, r := range dc.ExtraRecords {
		if r.Type != "" && r.Type != "A" && r.Type != "AAAA" {
			continue
		}
		if ip, err := netip.ParseAddr(r.Value); err == nil {
			m[normalizeName(r.Name)] = ip
		}
	}
	var domains []string
	for _, d := range dc.Domains {
		domains = append(domains, normalizeName(d))
	}
	// Right after a network change the node reports a DNS config with parts missing until
	// control reconnects. An empty list therefore means "unknown", not "removed": forgetting
	// known records and domains would make tailnet names unresolvable for as long as that takes.
	// fetched is only advanced for a config that carries something, so we ask again soon.
	if len(m) == 0 && len(domains) == 0 {
		return
	}
	if len(m) > 0 || len(t.extra) == 0 {
		t.extra = m
	}
	if len(domains) > 0 || len(t.domains) == 0 {
		t.domains = domains
	}
	t.fetched = time.Now()
}

func (t *nameTable) lookup(ctx context.Context, host string) (netip.Addr, bool) {
	host = normalizeName(host)
	if a, ok := t.static[host]; ok {
		return a, true
	}
	if t.fetch == nil {
		return netip.Addr{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refreshLocked(ctx, false)
	a, hit := t.extra[host]
	if !hit {
		t.refreshLocked(ctx, true)
		a, hit = t.extra[host]
	}
	return a, hit
}

// cached returns a name's address from the static hosts or the last known extra records,
// without asking the node.
func (t *nameTable) cached(name string) (netip.Addr, bool) {
	name = normalizeName(name)
	if a, ok := t.static[name]; ok {
		return a, true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.extra[name]
	return a, ok
}

// isTailnetName reports whether name is something the tailnet itself should answer.
func (t *nameTable) isTailnetName(ctx context.Context, name string) bool {
	name = normalizeName(name)
	if _, ok := t.static[name]; ok {
		return true
	}
	if t.fetch == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refreshLocked(ctx, false)
	check := func() bool {
		if _, ok := t.extra[name]; ok {
			return true
		}
		for _, d := range t.domains {
			if name == d || strings.HasSuffix(name, "."+d) {
				return true
			}
		}
		return false
	}
	if check() {
		return true
	}
	t.refreshLocked(ctx, true)
	return check()
}

// wrap returns a dial function that rewrites known names to their addresses.
func (t *nameTable) wrap(dial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if host, port, err := net.SplitHostPort(addr); err == nil {
			if _, err := netip.ParseAddr(host); err != nil {
				if ip, ok := t.lookup(ctx, host); ok {
					addr = net.JoinHostPort(ip.String(), port)
				}
			}
		}
		return dial(ctx, network, addr)
	}
}
