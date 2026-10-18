package tailnet

import (
	"context"
	goerrors "errors"
	"io"
	"net"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/signal"
	"github.com/xtls/xray-core/common/task"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/transport"
)

// Dialer opens connections inside the tailnet. *tsnet.Server satisfies it through
// DialFunc, tests substitute a plain dialer.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// DialFunc adapts a function (such as tsnet.Server.Dial) to Dialer.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// DialContext implements Dialer.
func (f DialFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

const dialTimeout = 20 * time.Second

// Handler is an Xray outbound.Handler that carries connections into the tailnet.
// Routing rules select it by tag; the connection target (IP or domain) arrives in
// the session context exactly as for the built-in freedom outbound.
type Handler struct {
	tag     string
	dialer  Dialer
	timeout policy.Timeout
}

// NewHandler creates a handler. timeouts may be nil to use Xray's defaults.
func NewHandler(tag string, dialer Dialer, pm policy.Manager) *Handler {
	h := &Handler{tag: tag, dialer: dialer, timeout: defaultTimeouts}
	if pm != nil {
		h.timeout = pm.ForLevel(0).Timeouts
	}
	return h
}

// defaultTimeouts mirror Xray's policy defaults.
var defaultTimeouts = policy.Timeout{
	Handshake:      4 * time.Second,
	ConnectionIdle: 300 * time.Second,
	UplinkOnly:     1 * time.Second,
	DownlinkOnly:   1 * time.Second,
}

// Tag implements outbound.Handler.
func (h *Handler) Tag() string { return h.tag }

// Start implements common.Runnable. The tailnet node itself is managed by the caller.
func (h *Handler) Start() error { return nil }

// Close implements common.Closable.
func (h *Handler) Close() error { return nil }

// SenderSettings implements outbound.Handler (not used).
func (h *Handler) SenderSettings() *serial.TypedMessage { return nil }

// ProxySettings implements outbound.Handler (not used).
func (h *Handler) ProxySettings() *serial.TypedMessage { return nil }

// Dispatch implements outbound.Handler with the same ownership rules as Xray's own
// proxyman handler: on error the link is interrupted, on success the writer is closed.
func (h *Handler) Dispatch(ctx context.Context, link *transport.Link) {
	err := h.process(ctx, link)
	if err != nil {
		cause := errors.Cause(err)
		if goerrors.Is(cause, io.EOF) || goerrors.Is(cause, io.ErrClosedPipe) || goerrors.Is(cause, context.Canceled) {
			err = nil
		}
	}
	if err != nil {
		err = errors.New("failed to process tailnet traffic").Base(err)
		session.SubmitOutboundErrorToOriginator(ctx, err)
		errors.LogInfo(ctx, err.Error())
		common.Interrupt(link.Writer)
	} else {
		common.Close(link.Writer)
	}
	common.Interrupt(link.Reader)
}

func (h *Handler) process(ctx context.Context, link *transport.Link) error {
	outbounds := session.OutboundsFromContext(ctx)
	if len(outbounds) == 0 {
		return errors.New("no outbound session")
	}
	ob := outbounds[len(outbounds)-1]
	if !ob.Target.IsValid() {
		return errors.New("target not specified")
	}
	dest := ob.Target
	if dest.Network != xnet.Network_TCP {
		return errors.New("tailnet outbound does not support ", dest.Network, " yet")
	}

	dctx, dcancel := context.WithTimeout(ctx, dialTimeout)
	conn, err := h.dialer.DialContext(dctx, "tcp", dest.NetAddr())
	dcancel()
	if err != nil {
		return errors.New("failed to open tailnet connection to ", dest).Base(err)
	}
	defer conn.Close()
	errors.LogInfo(ctx, "tailnet connection opened to ", dest)

	ctx, cancel := context.WithCancel(ctx)
	timer := signal.CancelAfterInactivity(ctx, cancel, h.timeout.ConnectionIdle)

	requestDone := func() error {
		defer timer.SetTimeout(h.timeout.DownlinkOnly)
		if err := buf.Copy(link.Reader, buf.NewWriter(conn), buf.UpdateActivity(timer)); err != nil {
			return errors.New("failed to process request").Base(err)
		}
		return nil
	}
	responseDone := func() error {
		defer timer.SetTimeout(h.timeout.UplinkOnly)
		if err := buf.Copy(buf.NewReader(conn), link.Writer, buf.UpdateActivity(timer)); err != nil {
			return errors.New("failed to process response").Base(err)
		}
		return nil
	}
	if err := task.Run(ctx, requestDone, task.OnSuccess(responseDone, task.Close(link.Writer))); err != nil {
		return errors.New("connection ends").Base(err)
	}
	return nil
}
