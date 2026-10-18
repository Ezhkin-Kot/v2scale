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

// nameTable resolves names that tsnet does not know before a dial.
//
// Headscale publishes extra_records as A records with an explicit type, which tsnet's
// dialer ignores (it only honours records with an empty type), so names such as
// svc.example.net fail with "no such host". The table consults the static hosts from the
// config, then the tailnet's DNS.ExtraRecords fetched from the node. Everything else,
// including ordinary MagicDNS peer names, goes to tsnet unchanged.
type nameTable struct {
	static map[string]netip.Addr
	fetch  func(ctx context.Context) ([]tailcfg.DNSRecord, error)

	mu      sync.Mutex
	extra   map[string]netip.Addr
	fetched time.Time
}

func newNameTable(hosts map[string]string, fetch func(context.Context) ([]tailcfg.DNSRecord, error)) *nameTable {
	t := &nameTable{static: map[string]netip.Addr{}, fetch: fetch}
	for h, ip := range hosts {
		if a, err := netip.ParseAddr(ip); err == nil {
			t.static[normalizeName(h)] = a
		}
	}
	return t
}

func normalizeName(h string) string { return strings.ToLower(strings.TrimSuffix(h, ".")) }

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
	a, hit := t.extra[host]
	age := time.Since(t.fetched)
	if age > namesRefresh || (!hit && age > namesMissWindow) {
		if recs, err := t.fetch(ctx); err == nil {
			m := map[string]netip.Addr{}
			for _, r := range recs {
				if r.Type != "" && r.Type != "A" && r.Type != "AAAA" {
					continue
				}
				if ip, err := netip.ParseAddr(r.Value); err == nil {
					m[normalizeName(r.Name)] = ip
				}
			}
			t.extra, t.fetched = m, time.Now()
			a, hit = m[host]
		}
	}
	return a, hit
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
