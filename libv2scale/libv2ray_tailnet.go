package libv2ray

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xtls/xray-core/core"

	"v2scale/libv2scale/tailnet"
)

// tailnetState is the V2Scale addition to CoreController: an embedded Tailscale node that
// serves the "tailnet" outbound tag.
type tailnetState struct {
	mu   sync.Mutex
	cfg  *tailnet.Config
	node *tailnet.Node
	// nodeID is the Identity of the config node was started with. A node outlives
	// StopLoopForReload so that a reload does not cost a control reconnect.
	nodeID string
}

// SetTailnetConfig sets the embedded tailnet node (JSON, see tailnet.Config) used from the
// next StartLoop on, or clears it when configJSON is empty. With a node configured, routing
// rules in the Xray config may send traffic to the outbound tag "tailnet".
func (x *CoreController) SetTailnetConfig(configJSON string) error {
	x.tailnet.mu.Lock()
	defer x.tailnet.mu.Unlock()
	if strings.TrimSpace(configJSON) == "" {
		x.tailnet.cfg = nil
		return nil
	}
	cfg, err := tailnet.ParseConfig(configJSON)
	if err != nil {
		return err
	}
	x.tailnet.cfg = cfg
	return nil
}

// TailnetStatus returns the node state as JSON:
// {"state":"stopped|starting|needs-login|running|...","auth_url":"...","ips":["100.64.0.2"]}.
func (x *CoreController) TailnetStatus() string {
	x.tailnet.mu.Lock()
	node := x.tailnet.node
	x.tailnet.mu.Unlock()

	out := struct {
		State   string   `json:"state"`
		AuthURL string   `json:"auth_url"`
		IPs     []string `json:"ips"`
	}{State: "stopped", IPs: []string{}}
	if node != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		st := node.Status(ctx)
		out.State, out.AuthURL = st.State, st.AuthURL
		for _, ip := range st.IPs {
			out.IPs = append(out.IPs, ip.String())
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// TailnetTest checks the running node and returns the result as JSON, see tailnet.TestResult.
// target is an optional "host:port" dialed through the tailnet; empty pings an online peer.
func (x *CoreController) TailnetTest(target string) string {
	x.tailnet.mu.Lock()
	node := x.tailnet.node
	x.tailnet.mu.Unlock()

	res := tailnet.TestResult{State: "stopped", Error: "tailnet is not running"}
	if node != nil {
		res = node.Test(context.Background(), strings.TrimSpace(target))
	}
	b, _ := json.Marshal(res)
	return string(b)
}

// startTailnet attaches the "tailnet" outbound to inst before the core starts. A node that is
// already running for the same config is reused (a core reload keeps it), otherwise it is
// brought up in the background. It closes a leftover node when tailnet is no longer configured.
func (x *CoreController) startTailnet(inst *core.Instance) error {
	x.tailnet.mu.Lock()
	defer x.tailnet.mu.Unlock()
	cfg := x.tailnet.cfg
	if cfg == nil {
		x.closeNodeLocked()
		return nil
	}
	if x.tailnet.node != nil && x.tailnet.nodeID != cfg.Identity() {
		x.closeNodeLocked()
	}
	node := x.tailnet.node
	if node == nil {
		var err error
		if node, err = tailnet.NewNode(*cfg); err != nil {
			return err
		}
		if err := node.Start(); err != nil {
			return fmt.Errorf("node start: %w", err)
		}
		x.tailnet.node, x.tailnet.nodeID = node, cfg.Identity()
	}
	if _, err := tailnet.AttachNode(context.Background(), inst, tailnet.DefaultTag, node); err != nil {
		return fmt.Errorf("attach outbound: %w", err)
	}
	return nil
}

func (x *CoreController) closeNodeLocked() {
	if x.tailnet.node != nil {
		x.tailnet.node.Close()
		x.tailnet.node, x.tailnet.nodeID = nil, ""
	}
}

// stopTailnet releases the node together with the core.
func (x *CoreController) stopTailnet() {
	x.tailnet.mu.Lock()
	defer x.tailnet.mu.Unlock()
	x.closeNodeLocked()
}

// StopLoopForReload stops the core like StopLoop but leaves the tailnet node running, so the
// next StartLoop with the same tailnet config reattaches to it. Use it to apply a network
// change; use StopLoop to really stop.
func (x *CoreController) StopLoopForReload() error {
	x.coreMutex.Lock()
	defer x.coreMutex.Unlock()

	if x.IsRunning {
		x.shutdownCore()
		x.CallbackHandler.Shutdown()
		x.CallbackHandler.OnEmitStatus(0, "Core stopped")
	}
	return nil
}
