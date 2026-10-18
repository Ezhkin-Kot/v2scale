package tailnet

import (
	"context"
	"errors"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/stats"
)

var errNoOutboundManager = errors.New("tailnet: xray instance has no outbound manager")

// DefaultTag is the outbound tag routing rules use to send traffic into the tailnet.
const DefaultTag = "tailnet"

// Attach registers a tailnet outbound handler with tag in the Xray instance. It must be
// called after core.New and may be called before or after Start. Traffic counters are
// registered under the standard names ("outbound>>>tag>>>traffic>>>uplink|downlink") and
// honour the same policy switches as Xray's own outbounds.
func Attach(ctx context.Context, inst *core.Instance, tag string, dialer Dialer) (*Handler, error) {
	pm, _ := inst.GetFeature(policy.ManagerType()).(policy.Manager)
	h := NewHandler(tag, dialer, pm)

	if sm, ok := inst.GetFeature(stats.ManagerType()).(stats.Manager); ok && pm != nil {
		sys := pm.ForSystem().Stats
		var up, down stats.Counter
		if sys.OutboundUplink {
			up, _ = sm.GetOrRegisterCounter("outbound>>>" + tag + ">>>traffic>>>uplink")
		}
		if sys.OutboundDownlink {
			down, _ = sm.GetOrRegisterCounter("outbound>>>" + tag + ">>>traffic>>>downlink")
		}
		h.SetCounters(up, down)
	}

	mgr, ok := inst.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok {
		return nil, errNoOutboundManager
	}
	if err := mgr.AddHandler(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// AttachNode is Attach for an embedded node: connections are dialed through the node and
// DNS queries to the MagicDNS address are answered by the node's resolver.
func AttachNode(ctx context.Context, inst *core.Instance, tag string, node *Node) (*Handler, error) {
	h, err := Attach(ctx, inst, tag, DialFunc(node.DialContext))
	if err != nil {
		return nil, err
	}
	h.SetDNS(node)
	return h, nil
}
