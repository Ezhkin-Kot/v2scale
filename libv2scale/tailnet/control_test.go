package tailnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeControl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // any answer proves reachability
	}))
	defer srv.Close()
	if err := probeControl(context.Background(), srv.URL, ""); err != nil {
		t.Errorf("reachable server: %v", err)
	}

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if err := probeControl(context.Background(), "http://"+addr, ""); err == nil {
		t.Error("closed port must be unreachable")
	}
}

func TestProbeControlThroughProxy(t *testing.T) {
	var seen atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.Host == "control.invalid:80" || r.Host == "control.invalid")
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	if err := probeControl(context.Background(), "http://control.invalid", proxy.URL); err != nil || !seen.Load() {
		t.Errorf("through proxy: err=%v seen=%v", err, seen.Load())
	}
}

func TestControlWatchFollowsReachabilityAndStopsWhenRunning(t *testing.T) {
	var w controlWatch
	if s, _ := w.get(); s != ControlUnknown {
		t.Fatalf("initial: %s", s)
	}
	var fail atomic.Bool
	fail.Store(true)
	var isRunning atomic.Bool
	var probes atomic.Int32
	probe := func(context.Context) error {
		probes.Add(1)
		if fail.Load() {
			return errors.New("connection refused")
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.run(ctx, 5*time.Millisecond, 5*time.Millisecond, probe, isRunning.Load)
	}()
	wait := func(want ControlState) string {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
			if s, e := w.get(); s == want {
				return e
			}
		}
		s, _ := w.get()
		t.Fatalf("state %s, want %s", s, want)
		return ""
	}
	if e := wait(ControlUnreachable); e != "connection refused" {
		t.Errorf("reason: %q", e)
	}
	fail.Store(false)
	wait(ControlOK)

	isRunning.Store(true)
	time.Sleep(30 * time.Millisecond)
	n := probes.Load()
	time.Sleep(30 * time.Millisecond)
	if probes.Load() != n {
		t.Error("a running node must not be probed")
	}
	cancel()
	<-done
}
