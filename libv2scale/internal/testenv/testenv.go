// Package testenv provides a hermetic Tailscale environment for tests: a control server
// with a local DERP/STUN, and helpers to run tsnet nodes against it. It is imported by
// tests only and never linked into the Android library.
package testenv

import (
	"context"
	"io"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"testing"

	"tailscale.com/ipn/store/mem"
	"tailscale.com/net/netns"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/tstest/integration"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/logger"
)

// MagicDNSDomain is the tailnet DNS domain of the test control server.
const MagicDNSDomain = "tail-scale.ts.net"

// StartControl runs the control server and returns its URL.
func StartControl(t testing.TB) (string, *testcontrol.Server) {
	t.Helper()
	netns.SetEnabled(false) // the host may run its own Tailscale
	t.Cleanup(func() { netns.SetEnabled(true) })

	derpMap := integration.RunDERPAndSTUN(t, logger.Discard, "127.0.0.1")
	control := &testcontrol.Server{
		DERPMap:        derpMap,
		DNSConfig:      &tailcfg.DNSConfig{Proxied: true, Domains: []string{MagicDNSDomain}},
		MagicDNSDomain: MagicDNSDomain,
		Logf:           t.Logf,
	}
	control.HTTPTestServer = httptest.NewUnstartedServer(control)
	control.HTTPTestServer.Start()
	t.Cleanup(control.HTTPTestServer.Close)
	return control.HTTPTestServer.URL, control
}

// StartNode brings up a plain tsnet node and returns it with its tailnet IPv4 address.
func StartNode(t testing.TB, ctx context.Context, controlURL, name string) (*tsnet.Server, netip.Addr) {
	t.Helper()
	s := &tsnet.Server{
		Dir:        filepath.Join(t.TempDir(), name),
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

// ServeEcho makes the node echo TCP connections on port.
func ServeEcho(t testing.TB, s *tsnet.Server, port string) {
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
