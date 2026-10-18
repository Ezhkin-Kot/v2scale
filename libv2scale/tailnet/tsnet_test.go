package tailnet

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/xtls/xray-core/features/policy"
	"tailscale.com/tsnet"
	"tailscale.com/tstest/integration/testcontrol"
	"v2scale/libv2scale/internal/testenv"
)

// Thin wrappers around the shared hermetic environment (see internal/testenv).
func startControl(t *testing.T) (string, *testcontrol.Server) { return testenv.StartControl(t) }

func startNode(t *testing.T, ctx context.Context, controlURL, name string) (*tsnet.Server, netip.Addr) {
	return testenv.StartNode(t, ctx, controlURL, name)
}

func serveEcho(t *testing.T, s *tsnet.Server, port string) { testenv.ServeEcho(t, s, port) }

const tsRules = `{"type":"field","ip":["100.64.0.0/10"],"outboundTag":"tailnet"},
                 {"type":"field","domain":["domain:tail-scale.ts.net"],"outboundTag":"tailnet"}`

// Experiments 1.3/1.4: a real tsnet node behind the Xray outbound carries TCP to a peer,
// addressed by tailnet IP and by MagicDNS name.
func TestXrayToTailnetTCP(t *testing.T) {
	controlURL, _ := startControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	server, serverIP := startNode(t, ctx, controlURL, "echo")
	serveEcho(t, server, "7")
	phone := startTestNode(t, ctx, controlURL, "phone", nil)

	inst, socks, err := newInstance(t, baseOutbounds, tsRules)
	if err != nil {
		t.Fatal(err)
	}
	pm, _ := inst.GetFeature(policy.ManagerType()).(policy.Manager)
	h := NewHandler("tailnet", DialFunc(phone.DialContext), pm)
	if err := outboundManager(t, inst).AddHandler(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}

	t.Run("by IP", func(t *testing.T) {
		got, err := viaSocks(t, socks, net.JoinHostPort(serverIP.String(), "7"))
		if err != nil || got != "hello" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("by MagicDNS name", func(t *testing.T) {
		got, err := viaSocks(t, socks, "echo.tail-scale.ts.net:7")
		if err != nil || got != "hello" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("unreachable peer fails cleanly", func(t *testing.T) {
		if got, err := viaSocks(t, socks, "100.64.200.200:7"); err == nil {
			t.Fatalf("expected a failure, got %q", got)
		}
	})
}

// waitDNS blocks until the node's resolver answers for name. WaitUp returns before the
// tailnet DNS configuration has been applied; until then the built-in resolver forwards
// every query to the host's resolvers (see docs/RESEARCH.md, DNS leak note).
func waitDNS(t *testing.T, ctx context.Context, n *Node, name string) {
	t.Helper()
	lc, err := n.server().LocalClient()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if raw, _, err := lc.QueryDNS(ctx, name, "A"); err == nil {
			if ips, _ := parseA(t, raw); len(ips) > 0 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("resolver never answered for %s", name)
}
