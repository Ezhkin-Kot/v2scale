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

// A probe that merely times out (a slow i2p tunnel) is "slow", and only a run of timeouts, or a
// definite failure, is "unreachable".
func TestControlWatchTreatsTimeoutsAsSlowFirst(t *testing.T) {
	cases := []struct {
		name string
		err  error
		// seen[i] is the state published after probe i+1 failed.
		seen []ControlState
	}{
		{"timeouts", context.DeadlineExceeded, []ControlState{ControlSlow, ControlSlow, ControlUnreachable, ControlUnreachable}},
		{"refused", errors.New("dial tcp 127.0.0.1:1: connect: connection refused"), []ControlState{ControlUnreachable, ControlUnreachable}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var w controlWatch
			var got []ControlState
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			probe := func(context.Context) error {
				// The state published by the previous round is visible when the next one starts.
				if len(got) > 0 || w.state != "" {
					s, _ := w.get()
					got = append(got, s)
				} else {
					got = append(got, "")
				}
				if len(got) == len(c.seen)+1 {
					cancel()
				}
				return c.err
			}
			go func() {
				defer close(done)
				w.run(ctx, time.Millisecond, time.Millisecond, probe, func() bool { return false })
			}()
			<-done
			// got[0] is the state before any probe (unset), got[i] the state after probe i.
			for i, want := range c.seen {
				if got[i+1] != want {
					t.Errorf("after probe %d: %q, want %q (all: %v)", i+1, got[i+1], want, got)
				}
			}
		})
	}
}
