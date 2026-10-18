package tailnet

import (
	"context"
	"net"
	"testing"
	"time"

	"tailscale.com/ipn/store/mem"

	"v2scale/libv2scale/internal/testenv"
)

func TestNodeTest(t *testing.T) {
	controlURL, _ := testenv.StartControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	server, serverIP := testenv.StartNode(t, ctx, controlURL, "echo")
	testenv.ServeEcho(t, server, "7")
	n := startTestNode(t, ctx, controlURL, "phone", nil)

	// The server is a peer of the phone only once the netmap has been delivered.
	var res TestResult
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if res = n.Test(ctx, ""); res.OK {
			break
		}
	}
	if !res.OK || res.State != "running" || res.Peer != "echo" || (res.Path != "direct" && res.Path != "relay") {
		t.Fatalf("peer ping: %+v", res)
	}

	res = n.Test(ctx, net.JoinHostPort(serverIP.String(), "7"))
	if !res.OK || res.Path != "tcp" || res.LatencyMS < 0 {
		t.Errorf("dial a listening port: %+v", res)
	}

	res = n.Test(ctx, net.JoinHostPort(serverIP.String(), "9")) // nothing listens there
	if res.OK || res.Error == "" {
		t.Errorf("dial a closed port must fail with a reason: %+v", res)
	}
}

func TestNodeTestBeforeStart(t *testing.T) {
	n, err := NewNode(Config{ControlURL: "http://127.0.0.1:1", StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res := n.Test(context.Background(), ""); res.OK || res.State != "stopped" || res.Error == "" {
		t.Errorf("not started: %+v", res)
	}
}

// The test must wait for a node that is still connecting instead of reporting it as failed.
func TestNodeTestWaitsForTheNodeToComeUp(t *testing.T) {
	controlURL, _ := testenv.StartControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	server, serverIP := testenv.StartNode(t, ctx, controlURL, "echo")
	testenv.ServeEcho(t, server, "7")

	n, err := NewNode(Config{ControlURL: controlURL, Hostname: "phone", StateDir: t.TempDir(), Store: new(mem.Store), Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	// No WaitUp: the node is still starting when the test begins.
	res := n.Test(ctx, net.JoinHostPort(serverIP.String(), "7"))
	if !res.OK || res.State != "running" {
		t.Fatalf("a starting node must be waited for: %+v", res)
	}
}
