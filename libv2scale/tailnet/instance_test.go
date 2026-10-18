package tailnet

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	"golang.org/x/net/proxy"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func echoServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	return l.Addr().String()
}

// recordingDialer stands in for tsnet: it records what the handler asked for and
// connects to the real echo server instead.
type recordingDialer struct {
	mu     sync.Mutex
	asked  []string
	target string
}

func (d *recordingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.asked = append(d.asked, network+" "+addr)
	d.mu.Unlock()
	var nd net.Dialer
	return nd.DialContext(ctx, "tcp", d.target)
}

func (d *recordingDialer) requests() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.asked...)
}

const baseOutbounds = `{"protocol":"freedom","tag":"direct"}`

// buildInstance loads a JSON config and creates (but does not start) an Xray instance.
func buildInstance(t *testing.T, cfg string) (*core.Instance, error) {
	t.Helper()
	pb, err := serial.LoadJSONConfig(strings.NewReader(cfg))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inst, err := core.New(pb)
	if err != nil {
		return nil, fmt.Errorf("core.New: %w", err)
	}
	t.Cleanup(func() { inst.Close() })
	return inst, nil
}

// newInstance builds (but does not start) an Xray instance with a SOCKS inbound.
func newInstance(t *testing.T, outbounds, rules string) (*core.Instance, string, error) {
	t.Helper()
	port := freePort(t)
	cfg := fmt.Sprintf(`{
	  "log": {"loglevel": "none"},
	  "inbounds": [{"tag":"in","listen":"127.0.0.1","port":%d,"protocol":"socks","settings":{"auth":"noauth","udp":false}}],
	  "outbounds": [%s],
	  "routing": {"domainStrategy":"AsIs","rules":[%s]}
	}`, port, outbounds, rules)
	inst, err := buildInstance(t, cfg)
	if err != nil {
		return nil, "", err
	}
	return inst, fmt.Sprintf("127.0.0.1:%d", port), nil
}

func newTestHandler(t *testing.T, inst *core.Instance, echo string) (*Handler, *recordingDialer) {
	t.Helper()
	d := &recordingDialer{target: echo}
	pm, _ := inst.GetFeature(policy.ManagerType()).(policy.Manager)
	return NewHandler("tailnet", d, pm), d
}

func outboundManager(t *testing.T, inst *core.Instance) outbound.Manager {
	t.Helper()
	m, ok := inst.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok {
		t.Fatal("no outbound manager")
	}
	return m
}

func viaSocks(t *testing.T, socks, target string) (string, error) {
	t.Helper()
	d, err := proxy.SOCKS5("tcp", socks, nil, &net.Dialer{Timeout: 3 * time.Second})
	if err != nil {
		return "", err
	}
	c, err := d.Dial("tcp", target)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("hello")); err != nil {
		return "", err
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

const tailnetRule = `{"type":"field","ip":["100.64.0.0/10"],"outboundTag":"tailnet"},
                     {"type":"field","domain":["domain:tailnet.test"],"outboundTag":"tailnet"}`

// Experiment 1.2a: a routing rule may reference a tag that does not exist yet, and the
// handler can be added after core.New but before Start.
func TestHandlerAddedBeforeStart(t *testing.T) {
	echo := echoServer(t)
	inst, socks, err := newInstance(t, baseOutbounds, tailnetRule)
	if err != nil {
		t.Fatalf("a rule for a not-yet-existing tag must load: %v", err)
	}
	h, dialer := newTestHandler(t, inst, echo)
	if err := outboundManager(t, inst).AddHandler(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}

	got, err := viaSocks(t, socks, "100.64.0.9:7")
	if err != nil || got != "hello" {
		t.Fatalf("via tailnet handler: %q, %v", got, err)
	}
	if reqs := dialer.requests(); len(reqs) != 1 || reqs[0] != "tcp 100.64.0.9:7" {
		t.Errorf("handler dial requests = %v", reqs)
	}

	// Unrelated traffic keeps using the first outbound and never touches the handler.
	got, err = viaSocks(t, socks, echo)
	if err != nil || got != "hello" {
		t.Fatalf("direct traffic: %q, %v", got, err)
	}
	if n := len(dialer.requests()); n != 1 {
		t.Errorf("direct traffic leaked into the tailnet handler (%d requests)", n)
	}
}

// Experiment 1.2b: adding the handler to an already running instance works too.
func TestHandlerAddedAfterStart(t *testing.T) {
	echo := echoServer(t)
	inst, socks, err := newInstance(t, baseOutbounds, tailnetRule)
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}
	h, _ := newTestHandler(t, inst, echo)
	if err := outboundManager(t, inst).AddHandler(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if got, err := viaSocks(t, socks, "100.64.0.9:7"); err != nil || got != "hello" {
		t.Fatalf("handler added after start: %q, %v", got, err)
	}
}

// Experiment 1.2c: a placeholder outbound with the same tag blocks AddHandler until it
// is removed, so the "placeholder + replace" approach is possible.
func TestTagConflictAndReplace(t *testing.T) {
	echo := echoServer(t)
	inst, socks, err := newInstance(t, baseOutbounds+`,{"protocol":"blackhole","tag":"tailnet"}`, tailnetRule)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := newTestHandler(t, inst, echo)
	mgr := outboundManager(t, inst)
	if err := mgr.AddHandler(context.Background(), h); err == nil {
		t.Fatal("AddHandler must refuse a duplicate tag")
	}
	if err := mgr.RemoveHandler(context.Background(), "tailnet"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddHandler(context.Background(), h); err != nil {
		t.Fatalf("add after remove: %v", err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}
	if got, err := viaSocks(t, socks, "100.64.0.9:7"); err != nil || got != "hello" {
		t.Fatalf("replaced handler: %q, %v", got, err)
	}
}

// Domain targets reach the handler unresolved, which is what lets tsnet resolve
// MagicDNS names itself.
func TestDomainTargetIsPassedUnresolved(t *testing.T) {
	echo := echoServer(t)
	inst, socks, err := newInstance(t, baseOutbounds, tailnetRule)
	if err != nil {
		t.Fatal(err)
	}
	h, dialer := newTestHandler(t, inst, echo)
	if err := outboundManager(t, inst).AddHandler(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}
	if got, err := viaSocks(t, socks, "svc.tailnet.test:7"); err != nil || got != "hello" {
		t.Fatalf("domain target: %q, %v", got, err)
	}
	if reqs := dialer.requests(); len(reqs) != 1 || reqs[0] != "tcp svc.tailnet.test:7" {
		t.Errorf("handler dial requests = %v, want the unresolved domain", reqs)
	}
}

// Documents what happens when a rule points at a tag with no handler: the connection
// must fail instead of silently going somewhere else.
func TestMissingHandlerDoesNotLeak(t *testing.T) {
	inst, socks, err := newInstance(t, baseOutbounds, tailnetRule)
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}
	if got, err := viaSocks(t, socks, "svc.tailnet.test:7"); err == nil {
		t.Logf("NOTE: with no handler the connection did not fail (got %q): traffic fell back to another outbound", got)
		t.Fail()
	}
}
