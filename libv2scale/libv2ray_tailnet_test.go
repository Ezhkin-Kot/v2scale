package libv2ray

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"v2scale/libv2scale/internal/testenv"
)

type testHandler struct{}

func (testHandler) Startup() int                 { return 0 }
func (testHandler) Shutdown() int                { return 0 }
func (testHandler) OnEmitStatus(int, string) int { return 0 }

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func xrayConfig(socksPort int) string {
	return fmt.Sprintf(`{
	  "log": {"loglevel": "none"},
	  "inbounds": [{"tag":"in","listen":"127.0.0.1","port":%d,"protocol":"socks","settings":{"auth":"noauth","udp":false}}],
	  "outbounds": [{"protocol":"freedom","tag":"direct"}],
	  "routing": {"domainStrategy":"AsIs","rules":[
	    {"type":"field","ip":["100.64.0.0/10"],"outboundTag":"tailnet"},
	    {"type":"field","domain":["domain:tail-scale.ts.net"],"outboundTag":"tailnet"}]}
	}`, socksPort)
}

func echoVia(socks, target string) (string, error) {
	d, err := proxy.SOCKS5("tcp", socks, nil, &net.Dialer{Timeout: 3 * time.Second})
	if err != nil {
		return "", err
	}
	c, err := d.Dial("tcp", target)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("hello")); err != nil {
		return "", err
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func tailnetState_(t *testing.T, x *CoreController) (state, authURL string, ips []string) {
	t.Helper()
	var st struct {
		State   string   `json:"state"`
		AuthURL string   `json:"auth_url"`
		IPs     []string `json:"ips"`
	}
	if err := json.Unmarshal([]byte(x.TailnetStatus()), &st); err != nil {
		t.Fatal(err)
	}
	return st.State, st.AuthURL, st.IPs
}

func waitRunning(t *testing.T, x *CoreController) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if s, _, ips := tailnetState_(t, x); s == "running" && len(ips) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("tailnet did not reach running: %s", x.TailnetStatus())
}

// The whole stack the app uses: CoreController with a tailnet config, a routing rule for
// the "tailnet" tag, and a stop that tears the node down.
func TestCoreControllerWithTailnet(t *testing.T) {
	controlURL, _ := testenv.StartControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	server, serverIP := testenv.StartNode(t, ctx, controlURL, "echo")
	testenv.ServeEcho(t, server, "7")

	x := NewCoreController(testHandler{})
	if s, _, _ := tailnetState_(t, x); s != "stopped" {
		t.Fatalf("status before start: %s", s)
	}
	if err := x.SetTailnetConfig(`{"control_url":"` + controlURL + `","state_dir":"` + t.TempDir() + `","hostname":"phone"}`); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	if err := x.StartLoop(xrayConfig(port), 0); err != nil {
		t.Fatal(err)
	}
	socks := fmt.Sprintf("127.0.0.1:%d", port)
	waitRunning(t, x)

	if got, err := echoVia(socks, net.JoinHostPort(serverIP.String(), "7")); err != nil || got != "hello" {
		t.Fatalf("by IP: %q, %v", got, err)
	}
	if got, err := echoVia(socks, "echo.tail-scale.ts.net:7"); err != nil || got != "hello" {
		t.Fatalf("by MagicDNS name: %q, %v", got, err)
	}

	if err := x.StopLoop(); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := tailnetState_(t, x); s != "stopped" {
		t.Errorf("status after stop: %s", s)
	}
	if _, err := echoVia(socks, net.JoinHostPort(serverIP.String(), "7")); err == nil {
		t.Error("the inbound must be gone after StopLoop")
	}
}

// A reload (network change) must not cost the node: same config and the same node object, still
// running while the core is down, reattached to the new core, and gone only on a real stop.
func TestReloadKeepsTailnetNode(t *testing.T) {
	controlURL, _ := testenv.StartControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	server, serverIP := testenv.StartNode(t, ctx, controlURL, "echo")
	testenv.ServeEcho(t, server, "7")

	x := NewCoreController(testHandler{})
	dir := t.TempDir()
	cfg := `{"control_url":"` + controlURL + `","state_dir":"` + dir + `","hostname":"phone"`
	if err := x.SetTailnetConfig(cfg + `,"auth_key":"tskey-x"}`); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	if err := x.StartLoop(xrayConfig(port), 0); err != nil {
		t.Fatal(err)
	}
	waitRunning(t, x)
	first := x.tailnet.node
	_, _, ips := tailnetState_(t, x)

	if err := x.StopLoopForReload(); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := tailnetState_(t, x); s != "running" {
		t.Errorf("node must keep running while the core is down: %s", s)
	}

	// The auth key is erased after registration; the app then reloads without it.
	if err := x.SetTailnetConfig(cfg + `}`); err != nil {
		t.Fatal(err)
	}
	port = freePort(t)
	if err := x.StartLoop(xrayConfig(port), 0); err != nil {
		t.Fatal(err)
	}
	if x.tailnet.node != first {
		t.Error("reload must reuse the node, not create another one")
	}
	if _, _, again := tailnetState_(t, x); len(again) == 0 || again[0] != ips[0] {
		t.Errorf("address changed over a reload: %v -> %v", ips, again)
	}
	if got, err := echoVia(fmt.Sprintf("127.0.0.1:%d", port), net.JoinHostPort(serverIP.String(), "7")); err != nil || got != "hello" {
		t.Fatalf("after reload: %q, %v", got, err)
	}

	// A different node (hostname) must not be mistaken for the running one.
	if err := x.StopLoopForReload(); err != nil {
		t.Fatal(err)
	}
	if err := x.SetTailnetConfig(`{"control_url":"` + controlURL + `","state_dir":"` + t.TempDir() + `","hostname":"tablet"}`); err != nil {
		t.Fatal(err)
	}
	if err := x.StartLoop(xrayConfig(freePort(t)), 0); err != nil {
		t.Fatal(err)
	}
	if x.tailnet.node == first {
		t.Error("changed config must start a new node")
	}
	if err := x.StopLoop(); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := tailnetState_(t, x); s != "stopped" {
		t.Errorf("StopLoop must release the node: %s", s)
	}
}

func TestSetTailnetConfigValidation(t *testing.T) {
	x := NewCoreController(testHandler{})
	if err := x.SetTailnetConfig(`{"control_url":"nope","state_dir":"/x"}`); err == nil {
		t.Error("invalid config must be rejected")
	}
	if err := x.SetTailnetConfig(`{"control_url":"http://h","state_dir":"/x"}`); err != nil {
		t.Errorf("valid config: %v", err)
	}
	if err := x.SetTailnetConfig(""); err != nil {
		t.Errorf("clearing: %v", err)
	}
}

// Without a tailnet config nothing changes: rules for the tailnet tag fail closed.
func TestCoreControllerWithoutTailnet(t *testing.T) {
	x := NewCoreController(testHandler{})
	port := freePort(t)
	if err := x.StartLoop(xrayConfig(port), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { x.StopLoop() })
	if s, _, _ := tailnetState_(t, x); s != "stopped" {
		t.Errorf("status: %s", s)
	}
	if got, err := echoVia(fmt.Sprintf("127.0.0.1:%d", port), "100.64.0.9:7"); err == nil {
		t.Errorf("traffic for the tailnet tag must fail without a tailnet, got %q", got)
	}
}

func TestTailnetTestWithoutNode(t *testing.T) {
	x := NewCoreController(testHandler{})
	var res struct {
		OK    bool   `json:"ok"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(x.TailnetTest("")), &res); err != nil {
		t.Fatal(err)
	}
	if res.OK || res.State != "stopped" || res.Error == "" {
		t.Errorf("no node: %+v", res)
	}
}
