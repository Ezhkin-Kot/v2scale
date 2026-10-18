package tailnet

import (
	"context"
	"io"
	"net"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/xtls/xray-core/features/policy"
	"tailscale.com/ipn/store/mem"
	"tailscale.com/net/netns"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/tstest/integration"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/logger"
)

// startControl runs a hermetic Tailscale control server with a local DERP/STUN, so the
// tests need no network access and no real tailnet.
func startControl(t *testing.T) (string, *testcontrol.Server) {
	t.Helper()
	netns.SetEnabled(false) // the host may run its own Tailscale
	t.Cleanup(func() { netns.SetEnabled(true) })

	derpMap := integration.RunDERPAndSTUN(t, logger.Discard, "127.0.0.1")
	control := &testcontrol.Server{
		DERPMap:        derpMap,
		DNSConfig:      &tailcfg.DNSConfig{Proxied: true},
		MagicDNSDomain: "tail-scale.ts.net",
		Logf:           t.Logf,
	}
	control.HTTPTestServer = httptest.NewUnstartedServer(control)
	control.HTTPTestServer.Start()
	t.Cleanup(control.HTTPTestServer.Close)
	return control.HTTPTestServer.URL, control
}

func startNode(t *testing.T, ctx context.Context, controlURL, name string) (*tsnet.Server, netip.Addr) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	s := &tsnet.Server{
		Dir:        dir,
		ControlURL: controlURL,
		Hostname:   name,
		Store:      new(mem.Store),
		Ephemeral:  true,
	}
	t.Cleanup(func() { s.Close() })
	st, err := s.Up(ctx)
	if err != nil {
		t.Fatalf("node %s did not come up: %v", name, err)
	}
	return s, st.TailscaleIPs[0]
}

func serveEcho(t *testing.T, s *tsnet.Server, port string) {
	t.Helper()
	ln, err := s.Listen("tcp", ":"+port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
}

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
