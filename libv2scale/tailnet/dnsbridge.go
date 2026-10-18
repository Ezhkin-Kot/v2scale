package tailnet

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/xtls/xray-core/common/buf"
	xerrors "github.com/xtls/xray-core/common/errors"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport"
	"golang.org/x/net/dns/dnsmessage"
)

// DNSResolver answers DNS queries for tailnet names. *Node implements it.
type DNSResolver interface {
	QueryDNS(ctx context.Context, name, qtype string) ([]byte, error)
}

// The addresses at which a tailnet node serves MagicDNS.
var magicDNSAddrs = [...]netip.Addr{
	netip.MustParseAddr("100.100.100.100"),
	netip.MustParseAddr("fd7a:115c:a1e0::53"),
}

const dnsIdle = 30 * time.Second

// SetDNS makes the handler answer queries sent to the MagicDNS address (100.100.100.100:53,
// UDP or TCP) itself, using r, instead of dialing that address through netstack. The
// netstack path forwards names to upstream servers, which would leak them; here a name
// outside the tailnet is answered REFUSED and nothing leaves the device.
func (h *Handler) SetDNS(r DNSResolver) { h.dns = r }

func isMagicDNS(dest xnet.Destination) bool {
	if dest.Port != 53 || !dest.Address.Family().IsIP() {
		return false
	}
	ip, ok := netip.AddrFromSlice(dest.Address.IP())
	if !ok {
		return false
	}
	ip = ip.Unmap()
	return ip == magicDNSAddrs[0] || ip == magicDNSAddrs[1]
}

// processDNS serves a DNS flow (one packet per query for UDP, length-prefixed for TCP).
func (h *Handler) processDNS(ctx context.Context, link *transport.Link, network xnet.Network) error {
	tr, ok := link.Reader.(buf.TimeoutReader)
	if !ok {
		return xerrors.New("tailnet DNS bridge needs a timeout-capable reader")
	}
	var pending []byte // TCP stream bytes not yet forming a full message
	for {
		mb, err := tr.ReadMultiBufferTimeout(dnsIdle)
		if err != nil {
			if errors.Is(err, buf.ErrReadTimeout) {
				return nil
			}
			return err
		}
		for _, b := range mb {
			if b == nil {
				continue
			}
			if network == xnet.Network_UDP {
				if err := h.writeDNS(link, h.answerDNS(ctx, b.Bytes()), false); err != nil {
					buf.ReleaseMulti(mb)
					return err
				}
				continue
			}
			pending = append(pending, b.Bytes()...)
		}
		buf.ReleaseMulti(mb)
		for network == xnet.Network_TCP && len(pending) >= 2 {
			n := int(binary.BigEndian.Uint16(pending))
			if len(pending) < 2+n {
				break
			}
			query := append([]byte(nil), pending[2:2+n]...)
			pending = pending[2+n:]
			if err := h.writeDNS(link, h.answerDNS(ctx, query), true); err != nil {
				return err
			}
		}
	}
}

func (h *Handler) writeDNS(link *transport.Link, resp []byte, framed bool) error {
	b := buf.New()
	if framed {
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(resp)))
		b.Write(l[:])
	}
	b.Write(resp)
	return link.Writer.WriteMultiBuffer(buf.MultiBuffer{b})
}

// answerDNS turns a DNS query into a response; it never returns nil.
func (h *Handler) answerDNS(ctx context.Context, query []byte) []byte {
	var p dnsmessage.Parser
	hdr, err := p.Start(query)
	if err != nil {
		return dnsErrorReply(hdr, nil, dnsmessage.RCodeFormatError)
	}
	q, err := p.Question()
	if err != nil {
		return dnsErrorReply(hdr, nil, dnsmessage.RCodeFormatError)
	}
	qtype, ok := dnsTypeName(q.Type)
	if !ok {
		return dnsErrorReply(hdr, &q, dnsmessage.RCodeNotImplemented)
	}
	raw, err := h.dns.QueryDNS(ctx, q.Name.String(), qtype)
	switch {
	case errors.Is(err, ErrNotTailnetName):
		return dnsErrorReply(hdr, &q, dnsmessage.RCodeRefused)
	case err != nil || len(raw) < 12:
		return dnsErrorReply(hdr, &q, dnsmessage.RCodeServerFailure)
	}
	// Reuse the node's answer but speak as the resolver the client asked.
	out := append([]byte(nil), raw...)
	binary.BigEndian.PutUint16(out[0:2], hdr.ID)
	if hdr.RecursionDesired {
		out[2] |= 0x01 // RD
	}
	out[3] |= 0x80 // RA
	return out
}

func dnsErrorReply(hdr dnsmessage.Header, q *dnsmessage.Question, rc dnsmessage.RCode) []byte {
	resp := dnsmessage.Header{ID: hdr.ID, Response: true, RecursionDesired: hdr.RecursionDesired, RecursionAvailable: true, RCode: rc}
	b := dnsmessage.NewBuilder(nil, resp)
	if q != nil {
		b.StartQuestions()
		b.Question(*q)
	}
	out, err := b.Finish()
	if err != nil {
		return []byte{0, 0, 0x81, 0x82, 0, 0, 0, 0, 0, 0, 0, 0}
	}
	return out
}

func dnsTypeName(t dnsmessage.Type) (string, bool) {
	switch t {
	case dnsmessage.TypeA, dnsmessage.TypeAAAA, dnsmessage.TypeCNAME, dnsmessage.TypeMX,
		dnsmessage.TypeNS, dnsmessage.TypePTR, dnsmessage.TypeSOA, dnsmessage.TypeSRV, dnsmessage.TypeTXT:
		return strings.TrimPrefix(t.String(), "Type"), true
	}
	return "", false
}
