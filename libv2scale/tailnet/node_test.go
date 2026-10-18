package tailnet

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"tailscale.com/ipn/store/mem"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

func startTestNode(t *testing.T, ctx context.Context, controlURL, name string, hosts map[string]string) *Node {
	t.Helper()
	n, err := NewNode(Config{
		ControlURL: controlURL,
		Hostname:   name,
		StateDir:   filepath.Join(t.TempDir(), name),
		Hosts:      hosts,
		Store:      new(mem.Store),
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	if err := n.WaitUp(ctx); err != nil {
		t.Fatalf("node %s did not come up: %v", name, err)
	}
	return n
}

func roundTrip(t *testing.T, c net.Conn) string {
	t.Helper()
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	return string(buf)
}

func TestNodeLifecycle(t *testing.T) {
	controlURL, _ := startControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	n, err := NewNode(Config{ControlURL: controlURL, Hostname: "lc", StateDir: t.TempDir(), Store: new(mem.Store), Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	if st := n.Status(ctx); st.State != "stopped" {
		t.Errorf("before Start: %+v", st)
	}
	if _, err := n.DialContext(ctx, "tcp", "100.64.0.1:1"); err == nil {
		t.Error("dial before Start must fail")
	}
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	if err := n.Start(); err == nil {
		t.Error("second Start must fail")
	}
	if err := n.WaitUp(ctx); err != nil {
		t.Fatal(err)
	}
	if st := n.Status(ctx); st.State != "running" || len(st.IPs) == 0 {
		t.Errorf("after WaitUp: %+v", st)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if st := n.Status(ctx); st.State != "stopped" {
		t.Errorf("after Close: %+v", st)
	}
	if err := n.Close(); err != nil {
		t.Errorf("Close must be idempotent: %v", err)
	}
}

// Headscale publishes extra_records as A records with an explicit type. tsnet alone cannot
// dial such names; the Node can.
func TestNodeResolvesExtraRecordsAndStaticHosts(t *testing.T) {
	controlURL, control := startControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	server, serverIP := startNode(t, ctx, controlURL, "echo")
	serveEcho(t, server, "7")
	control.AddDNSRecords(tailcfg.DNSRecord{Name: "svc.example.test", Type: "A", Value: serverIP.String()})

	phone := startTestNode(t, ctx, controlURL, "phone", map[string]string{"static.example.test": serverIP.String()})

	t.Run("extra record with type A", func(t *testing.T) {
		c, err := phone.DialContext(ctx, "tcp", "svc.example.test:7")
		if err != nil {
			t.Fatal(err)
		}
		if got := roundTrip(t, c); got != "hello" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("static host from config", func(t *testing.T) {
		c, err := phone.DialContext(ctx, "tcp", "static.example.test:7")
		if err != nil {
			t.Fatal(err)
		}
		if got := roundTrip(t, c); got != "hello" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("MagicDNS peer name", func(t *testing.T) {
		c, err := phone.DialContext(ctx, "tcp", "echo.tail-scale.ts.net:7")
		if err != nil {
			t.Fatal(err)
		}
		if got := roundTrip(t, c); got != "hello" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("tsnet alone cannot dial type A extra records (documents the quirk)", func(t *testing.T) {
		raw := &tsnet.Server{Dir: t.TempDir(), ControlURL: controlURL, Hostname: "raw", Store: new(mem.Store), Ephemeral: true}
		t.Cleanup(func() { raw.Close() })
		if _, err := raw.Up(ctx); err != nil {
			t.Fatal(err)
		}
		dctx, dcancel := context.WithTimeout(ctx, 5*time.Second)
		defer dcancel()
		c, err := raw.Dial(dctx, "tcp", "svc.example.test:7")
		if err == nil {
			c.Close()
			t.Log("tsnet now resolves type A extra records itself; the name table may no longer be needed")
		}
	})
}
