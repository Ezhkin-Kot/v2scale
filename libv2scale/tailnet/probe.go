package tailnet

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"time"

	"tailscale.com/tailcfg"
)

// TestResult is the outcome of Node.Test.
type TestResult struct {
	OK        bool   `json:"ok"`
	State     string `json:"state"`
	LatencyMS int64  `json:"latency_ms"`
	// Path is "direct", "relay" (through a DERP server) or "tcp" for a dialed target.
	Path string `json:"path,omitempty"`
	// Peer is the host that was pinged or dialed.
	Peer  string `json:"peer,omitempty"`
	Error string `json:"error,omitempty"`
}

const (
	testTimeout    = 10 * time.Second
	waitForNode    = 60 * time.Second
	perPingTimeout = 3 * time.Second
	maxPingPeers   = 3
)

// Test checks that the node works. With a target ("host:port") it dials it through the tailnet
// exactly like routed traffic would, which also exercises name resolution and ACLs. Without one it
// pings the first online peer at the WireGuard layer, which shows reachability and whether the
// path is direct or relayed but says nothing about ACLs.
//
// A node that is still connecting is waited for (up to waitForNode) instead of failing the test:
// right after the VPN starts, getting the network map can take a minute on a slow control path.
func (n *Node) Test(ctx context.Context, target string) TestResult {
	st := n.waitRunning(ctx, waitForNode)
	res := TestResult{State: st.State}
	if st.State != "running" {
		res.Error = notReadyMessage(st.State)
		if c, e := n.Control(); c == ControlUnreachable {
			res.Error += ", control server unreachable (" + e + ")"
		}
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	if target != "" {
		return n.testDial(ctx, target, res)
	}
	return n.testPing(ctx, res)
}

// waitRunning returns the node status once it is running, or the last status after limit. A node that
// was never started is not waited for.
func (n *Node) waitRunning(ctx context.Context, limit time.Duration) Status {
	deadline := time.Now().Add(limit)
	for {
		sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		st := n.Status(sctx)
		cancel()
		if st.State == "running" || st.State == "stopped" || time.Now().After(deadline) || ctx.Err() != nil {
			return st
		}
		select {
		case <-ctx.Done():
			return st
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (n *Node) testDial(ctx context.Context, target string, res TestResult) TestResult {
	res.Peer, res.Path = target, "tcp"
	start := time.Now()
	c, err := n.DialContext(ctx, "tcp", target)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	c.Close()
	res.OK, res.LatencyMS = true, time.Since(start).Milliseconds()
	return res
}

func (n *Node) testPing(ctx context.Context, res TestResult) TestResult {
	s := n.server()
	lc, err := s.LocalClient()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	st, err := lc.Status(ctx)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	// Prefer peers the control server reports online; without that information (or when none
	// is online) fall back to the rest, bounded so a dead peer cannot eat the whole budget.
	var online, rest []netip.Addr
	names := map[netip.Addr]string{}
	for _, p := range st.Peer {
		if len(p.TailscaleIPs) == 0 {
			continue
		}
		ip := p.TailscaleIPs[0]
		names[ip] = p.HostName
		if p.Online {
			online = append(online, ip)
		} else {
			rest = append(rest, ip)
		}
	}
	for _, l := range [][]netip.Addr{online, rest} {
		sort.Slice(l, func(i, j int) bool { return l[i].Less(l[j]) })
	}
	peers := append(online, rest...)
	if len(peers) == 0 {
		res.Error = "the tailnet has no other nodes to ping"
		return res
	}
	if len(peers) > maxPingPeers {
		peers = peers[:maxPingPeers]
	}

	var lastErr string
	for _, ip := range peers {
		pctx, cancel := context.WithTimeout(ctx, perPingTimeout)
		pr, err := lc.Ping(pctx, ip, tailcfg.PingDisco)
		cancel()
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if pr.Err != "" {
			lastErr = pr.Err
			continue
		}
		// The first ping includes setting up the path; a second one shows what traffic will see.
		pctx2, cancel2 := context.WithTimeout(ctx, perPingTimeout)
		if again, err := lc.Ping(pctx2, ip, tailcfg.PingDisco); err == nil && again.Err == "" && again.LatencySeconds < pr.LatencySeconds {
			pr = again
		}
		cancel2()
		res.OK, res.Peer = true, names[ip]
		res.LatencyMS = int64(pr.LatencySeconds * 1000)
		res.Path = "relay"
		if pr.Endpoint != "" {
			res.Path = "direct"
		}
		return res
	}
	res.Error = fmt.Sprintf("no peer answered (%s)", lastErr)
	return res
}

// notReadyMessage explains why the node is not usable in words a user can act on. The raw backend
// states ("nostate", "starting") say nothing to them.
func notReadyMessage(state string) string {
	switch state {
	case "stopped":
		return "the node is not running"
	case "needs-login":
		return "login is required"
	default:
		return "the node is still connecting"
	}
}
