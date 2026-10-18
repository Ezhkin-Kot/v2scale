package tailnet

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/features/policy"
	"golang.org/x/net/proxy"
	"tailscale.com/ipn/store/mem"
)

type stressEnv struct {
	socks    string
	serverIP string
}

// newStress builds: SOCKS inbound -> Xray -> tailnet handler -> tsnet -> echo peer.
func newStress(t *testing.T, ctx context.Context) stressEnv {
	t.Helper()
	controlURL, _ := startControl(t)
	server, serverIP := startNode(t, ctx, controlURL, "echo")
	serveEcho(t, server, "7")
	phone := startTestNode(t, ctx, controlURL, "phone", nil)
	inst, socks, err := newInstance(t, baseOutbounds, tsRules)
	if err != nil {
		t.Fatal(err)
	}
	pm, _ := inst.GetFeature(policy.ManagerType()).(policy.Manager)
	if err := outboundManager(t, inst).AddHandler(ctx, NewHandler("tailnet", DialFunc(phone.DialContext), pm)); err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}
	return stressEnv{socks: socks, serverIP: serverIP.String()}
}

func (e stressEnv) dial(t *testing.T) net.Conn {
	t.Helper()
	d, err := proxy.SOCKS5("tcp", e.socks, nil, &net.Dialer{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c, err := d.Dial("tcp", net.JoinHostPort(e.serverIP, "7"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A large transfer must arrive intact, and many connections must work at once.
func TestBulkTransferIntegrityAndConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	env := newStress(t, ctx)

	const size = 16 << 20
	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(payload)

	start := time.Now()
	c := env.dial(t)
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Minute))
	go func() {
		c.Write(payload)
	}()
	h := sha256.New()
	if _, err := io.CopyN(h, c, size); err != nil {
		t.Fatalf("reading echo: %v", err)
	}
	var got [32]byte
	copy(got[:], h.Sum(nil))
	if got != want {
		t.Fatal("echoed data differs from the original")
	}
	el := time.Since(start)
	t.Logf("echoed %d MiB both ways in %v (%.1f MiB/s each way)", size>>20, el.Round(time.Millisecond), float64(size>>20)/el.Seconds())

	// 40 parallel connections with distinct payloads.
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := proxy.SOCKS5("tcp", env.socks, nil, &net.Dialer{Timeout: 10 * time.Second})
			if err != nil {
				errs <- err
				return
			}
			cc, err := d.Dial("tcp", net.JoinHostPort(env.serverIP, "7"))
			if err != nil {
				errs <- err
				return
			}
			defer cc.Close()
			cc.SetDeadline(time.Now().Add(30 * time.Second))
			msg := bytes.Repeat([]byte{byte(i)}, 4096)
			if _, err := cc.Write(msg); err != nil {
				errs <- err
				return
			}
			back := make([]byte, len(msg))
			if _, err := io.ReadFull(cc, back); err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(msg, back) {
				errs <- io.ErrUnexpectedEOF
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("parallel connection: %v", err)
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("heap in use after the run: %d MiB, goroutines: %d", ms.HeapInuse>>20, runtime.NumGoroutine())
}

// Starting and stopping nodes repeatedly must not accumulate goroutines.
func TestNodeRestartDoesNotLeakGoroutines(t *testing.T) {
	controlURL, _ := startControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cycle := func(i int) {
		n, err := NewNode(Config{ControlURL: controlURL, Hostname: "leak", StateDir: filepath.Join(t.TempDir(), "n"), Store: new(mem.Store), Ephemeral: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := n.Start(); err != nil {
			t.Fatal(err)
		}
		if err := n.WaitUp(ctx); err != nil {
			t.Fatal(err)
		}
		n.Close()
	}
	cycle(0) // warm up lazily started package-level goroutines
	settle := func() int {
		deadline := time.Now().Add(10 * time.Second)
		last := runtime.NumGoroutine()
		for time.Now().Before(deadline) {
			time.Sleep(300 * time.Millisecond)
			cur := runtime.NumGoroutine()
			if cur == last {
				break
			}
			last = cur
		}
		return last
	}
	before := settle()
	for i := 1; i <= 4; i++ {
		cycle(i)
	}
	after := settle()
	t.Logf("goroutines before: %d, after 4 start/stop cycles: %d", before, after)
	if after > before+8 {
		t.Errorf("goroutines grow with restarts: %d -> %d", before, after)
	}
}
