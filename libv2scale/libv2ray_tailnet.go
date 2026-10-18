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

// startTailnet brings the node up in the background and registers its outbound before the
// core starts. It is a no-op without a tailnet config.
func (x *CoreController) startTailnet(inst *core.Instance) error {
	x.tailnet.mu.Lock()
	cfg := x.tailnet.cfg
	x.tailnet.mu.Unlock()
	if cfg == nil {
		return nil
	}
	node, err := tailnet.NewNode(*cfg)
	if err != nil {
		return err
	}
	if err := node.Start(); err != nil {
		return fmt.Errorf("node start: %w", err)
	}
	if _, err := tailnet.AttachNode(context.Background(), inst, tailnet.DefaultTag, node); err != nil {
		node.Close()
		return fmt.Errorf("attach outbound: %w", err)
	}
	x.tailnet.mu.Lock()
	x.tailnet.node = node
	x.tailnet.mu.Unlock()
	return nil
}

func (x *CoreController) stopTailnet() {
	x.tailnet.mu.Lock()
	node := x.tailnet.node
	x.tailnet.node = nil
	x.tailnet.mu.Unlock()
	if node != nil {
		node.Close()
	}
}
