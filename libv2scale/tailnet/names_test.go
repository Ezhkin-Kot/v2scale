package tailnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
	"tailscale.com/tailcfg"
)

func TestNameTable(t *testing.T) {
	calls := 0
	recs := []tailcfg.DNSRecord{
		{Name: "Svc.Example.Net.", Type: "A", Value: "100.64.0.2"}, // Headscale style, explicit type
		{Name: "plain.example.net", Value: "100.64.0.3"},           // empty type
		{Name: "v6.example.net", Type: "AAAA", Value: "fd7a::2"},
		{Name: "txt.example.net", Type: "TXT", Value: "hello"},
		{Name: "bad.example.net", Type: "A", Value: "not-an-ip"},
	}
	tbl := newNameTable(map[string]string{"Pinned.Example.Net": "100.64.9.9"},
		func(context.Context) (*tailcfg.DNSConfig, error) {
			calls++
			return &tailcfg.DNSConfig{ExtraRecords: recs, Domains: []string{"Tailnet.Example.Net."}}, nil
		})
	ctx := context.Background()

	for name, want := range map[string]string{
		"svc.example.net":    "100.64.0.2",
		"SVC.example.net.":   "100.64.0.2",
		"plain.example.net":  "100.64.0.3",
		"v6.example.net":     "fd7a::2",
		"pinned.example.net": "100.64.9.9",
	} {
		got, ok := tbl.lookup(ctx, name)
		if !ok || got.String() != want {
			t.Errorf("lookup(%q) = %v, %v; want %s", name, got, ok, want)
		}
	}
	for _, name := range []string{"txt.example.net", "bad.example.net", "unknown.example.net"} {
		if got, ok := tbl.lookup(ctx, name); ok {
			t.Errorf("lookup(%q) should miss, got %v", name, got)
		}
	}
	if calls != 1 {
		t.Errorf("records fetched %d times, want 1 (cached, and misses are throttled)", calls)
	}
}

func TestIsTailnetName(t *testing.T) {
	tbl := newNameTable(map[string]string{"pinned.example": "100.64.9.9"},
		func(context.Context) (*tailcfg.DNSConfig, error) {
			return &tailcfg.DNSConfig{
				Domains:      []string{"tailnet.example.net."},
				ExtraRecords: []tailcfg.DNSRecord{{Name: "svc.corp.example", Type: "A", Value: "100.64.0.2"}},
			}, nil
		})
	ctx := context.Background()
	for name, want := range map[string]bool{
		"peer.tailnet.example.net.": true,  // under a tailnet domain
		"TAILNET.example.net":       true,  // the domain itself
		"svc.corp.example.":         true,  // extra record
		"pinned.example":            true,  // static host
		"example.com.":              false, // ordinary internet name
		"nottailnet.example.net.":   false, // suffix must be a whole label
		"evil.net.":                 false,
	} {
		if got := tbl.isTailnetName(ctx, name); got != want {
			t.Errorf("isTailnetName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNameTableStaticWinsAndSurvivesFetchErrors(t *testing.T) {
	tbl := newNameTable(map[string]string{"a.example": "100.64.1.1"},
		func(context.Context) (*tailcfg.DNSConfig, error) { return nil, errors.New("no netmap yet") })
	if ip, ok := tbl.lookup(context.Background(), "a.example"); !ok || ip.String() != "100.64.1.1" {
		t.Errorf("static host: %v %v", ip, ok)
	}
	if _, ok := tbl.lookup(context.Background(), "b.example"); ok {
		t.Error("unknown name must miss when the fetch fails")
	}
}

func TestNameTableWrapRewritesOnlyKnownNames(t *testing.T) {
	var got []string
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		got = append(got, addr)
		return nil, errors.New("stop")
	}
	tbl := newNameTable(map[string]string{"svc.example.net": "100.64.0.2"}, nil)
	d := tbl.wrap(dial)
	for _, addr := range []string{"svc.example.net:443", "peer.tailnet.net:22", "100.64.0.9:80", "[fd7a::1]:80"} {
		d(context.Background(), "tcp", addr)
	}
	want := []string{"100.64.0.2:443", "peer.tailnet.net:22", "100.64.0.9:80", "[fd7a::1]:80"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dial %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// After a network change the node reports an empty DNS config until control reconnects.
// That must not make the tailnet forget its names.
func TestNameTableKeepsRecordsWhenConfigGoesEmpty(t *testing.T) {
	empty := false
	tbl := newNameTable(nil, func(context.Context) (*tailcfg.DNSConfig, error) {
		if empty {
			return &tailcfg.DNSConfig{}, nil
		}
		return &tailcfg.DNSConfig{
			Domains:      []string{"ts.example"},
			ExtraRecords: []tailcfg.DNSRecord{{Name: "svc.example.net", Type: "A", Value: "100.64.0.2"}},
		}, nil
	})
	ctx := context.Background()
	if !tbl.isTailnetName(ctx, "svc.example.net") {
		t.Fatal("record must be known before the config disappears")
	}
	empty = true
	tbl.mu.Lock()
	tbl.fetched = tbl.fetched.Add(-2 * namesRefresh) // force a reload
	tbl.mu.Unlock()
	if !tbl.isTailnetName(ctx, "svc.example.net") || !tbl.isTailnetName(ctx, "peer.ts.example") {
		t.Error("empty config must not erase known records and domains")
	}
	if a, ok := tbl.cached("SVC.example.net."); !ok || a.String() != "100.64.0.2" {
		t.Errorf("cached: %v %v", a, ok)
	}
	if tbl.isTailnetName(ctx, "example.com") {
		t.Error("unrelated name must stay outside the tailnet")
	}
}

func TestSynthesizeAnswer(t *testing.T) {
	parse := func(raw []byte) dnsmessage.Message {
		var m dnsmessage.Message
		if err := m.Unpack(raw); err != nil {
			t.Fatal(err)
		}
		return m
	}
	v4 := netip.MustParseAddr("100.64.0.2")

	raw, ok := synthesizeAnswer("svc.example.net.", "A", v4)
	if !ok {
		t.Fatal("A must be answerable")
	}
	m := parse(raw)
	if !m.Authoritative || m.RCode != dnsmessage.RCodeSuccess || len(m.Answers) != 1 {
		t.Fatalf("A answer: %+v", m.Header)
	}
	if got := m.Answers[0].Body.(*dnsmessage.AResource).A; got != [4]byte{100, 64, 0, 2} {
		t.Errorf("address: %v", got)
	}
	if !isLocalAnswer(raw) {
		t.Error("a synthesized answer must count as the node's own zone")
	}

	raw, ok = synthesizeAnswer("svc.example.net", "AAAA", v4)
	if !ok || len(parse(raw).Answers) != 0 || parse(raw).RCode != dnsmessage.RCodeSuccess {
		t.Error("AAAA for an IPv4 name must be an empty NOERROR (NODATA)")
	}

	raw, ok = synthesizeAnswer("svc.example.net", "AAAA", netip.MustParseAddr("fd7a:115c:a1e0::2"))
	if !ok || len(parse(raw).Answers) != 1 {
		t.Error("AAAA for an IPv6 name must carry the address")
	}

	if _, ok := synthesizeAnswer("svc.example.net", "MX", v4); ok {
		t.Error("only A and AAAA are synthesized")
	}
}

func TestNameTableKeepsEachListWhenOnlyOneGoesEmpty(t *testing.T) {
	phase := 0
	tbl := newNameTable(nil, func(context.Context) (*tailcfg.DNSConfig, error) {
		if phase == 0 {
			return &tailcfg.DNSConfig{
				Domains:      []string{"ts.example"},
				ExtraRecords: []tailcfg.DNSRecord{{Name: "svc.example.net", Type: "A", Value: "100.64.0.2"}},
			}, nil
		}
		return &tailcfg.DNSConfig{Domains: []string{"ts.example"}}, nil // records not delivered yet
	})
	ctx := context.Background()
	tbl.isTailnetName(ctx, "svc.example.net")
	phase = 1
	tbl.mu.Lock()
	tbl.fetched = tbl.fetched.Add(-2 * namesRefresh)
	tbl.mu.Unlock()
	if !tbl.isTailnetName(ctx, "svc.example.net") {
		t.Error("extra records must survive a config that only carries domains")
	}
}
