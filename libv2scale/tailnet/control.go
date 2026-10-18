package tailnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ControlState is what is known about the reachability of the control server.
type ControlState string

const (
	// ControlUnknown: nothing probed yet, or the node is not waiting for control.
	ControlUnknown ControlState = "unknown"
	// ControlOK: the control server answered an HTTP request.
	ControlOK ControlState = "ok"
	// ControlSlow: probes time out. A slow path (i2p tunnels still being built, a loaded proxy) looks
	// like this for a while and then works, so it must not be reported as a failure yet.
	ControlSlow ControlState = "slow"
	// ControlUnreachable: the last probe failed outright (no route, refused, DNS), or probes kept
	// timing out for controlSlowLimit rounds in a row.
	ControlUnreachable ControlState = "unreachable"
)

const (
	controlProbeInterval = 5 * time.Second
	controlProbeTimeout  = 15 * time.Second
	controlRunningPause  = 15 * time.Second
	// controlSlowLimit is how many timeouts in a row still count as "slow" before they count as down.
	controlSlowLimit = 3
)

// controlWatch probes the control server in the background while the node is not yet running.
// "Connecting" alone does not tell a slow tunnel (i2pd, a proxy) from a server that never answers,
// and the user cannot fix either blindly.
type controlWatch struct {
	mu     sync.Mutex
	state  ControlState
	err    string
	cancel context.CancelFunc
	done   chan struct{}
}

func (w *controlWatch) set(s ControlState, err string) {
	w.mu.Lock()
	w.state, w.err = s, err
	w.mu.Unlock()
}

func (w *controlWatch) get() (ControlState, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == "" {
		return ControlUnknown, ""
	}
	return w.state, w.err
}

// run probes every interval until ctx ends. While running() is true the node has the netmap and
// control is by definition fine, so nothing is probed.
func (w *controlWatch) run(ctx context.Context, interval, pause time.Duration, probe func(context.Context) error, running func() bool) {
	timeouts := 0
	for {
		wait := interval
		if running() {
			timeouts = 0
			w.set(ControlOK, "")
			wait = pause
		} else if err := probe(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			if isTimeout(err) {
				timeouts++
			} else {
				timeouts = controlSlowLimit // refused, no route, DNS failure: a definite answer
			}
			if timeouts >= controlSlowLimit {
				w.set(ControlUnreachable, err.Error())
			} else {
				w.set(ControlSlow, err.Error())
			}
		} else {
			timeouts = 0
			w.set(ControlOK, "")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (w *controlWatch) stop() {
	w.mu.Lock()
	cancel, done := w.cancel, w.done
	w.cancel, w.done = nil, nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (n *Node) startControlWatch() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	n.ctrl.mu.Lock()
	n.ctrl.cancel, n.ctrl.done = cancel, done
	n.ctrl.mu.Unlock()
	probe := func(ctx context.Context) error { return probeControl(ctx, n.cfg.ControlURL, n.cfg.ControlProxy) }
	running := func() bool {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return n.Status(c).State == "running"
	}
	go func() {
		defer close(done)
		n.ctrl.run(ctx, controlProbeInterval, controlRunningPause, probe, running)
	}()
}

// Control reports whether the control server was reachable at the last probe, and why not.
func (n *Node) Control() (ControlState, string) { return n.ctrl.get() }

// probeControl makes one HTTP request to the control server (through proxy if set). Any answer, even
// an error status, means the server is reachable. /key is the endpoint both Tailscale and Headscale
// serve for clients to learn the server's public key.
func probeControl(ctx context.Context, controlURL, proxy string) error {
	tr := &http.Transport{DisableKeepAlives: true}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	defer tr.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, controlProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(controlURL, "/")+"/key?v=1", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return shortErr(err)
	}
	resp.Body.Close()
	return nil
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())
}

// shortErr drops the URL that net/http puts in front of the cause, it is already known to the user.
func shortErr(err error) error {
	if ue, ok := err.(*url.Error); ok {
		return ue.Err
	}
	return err
}
