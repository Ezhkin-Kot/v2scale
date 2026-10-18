package tailnet

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

type fakeResolver struct {
	calls  []string
	answer map[string][]byte
	err    error
}

func (f *fakeResolver) QueryDNS(ctx context.Context, name, qtype string) ([]byte, error) {
	f.calls = append(f.calls, qtype+" "+name)
	if f.err != nil {
		return nil, f.err
	}
	if a, ok := f.answer[name]; ok {
		return a, nil
	}
	return nil, ErrNotTailnetName
}

// nodeAnswer fabricates the kind of response the node's resolver returns (ID of its own).
func nodeAnswer(name string, ip [4]byte) []byte {
	n := dnsmessage.MustNewName(name)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 0xbeef, Response: true, Authoritative: true})
	b.StartQuestions()
	b.Question(dnsmessage.Question{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET})
	b.StartAnswers()
	b.AResource(dnsmessage.ResourceHeader{Name: n, Class: dnsmessage.ClassINET, TTL: 60}, dnsmessage.AResource{A: ip})
	out, _ := b.Finish()
	return out
}

func TestDNSBridgeAnswers(t *testing.T) {
	res := &fakeResolver{answer: map[string][]byte{"echo.tail-scale.ts.net.": nodeAnswer("echo.tail-scale.ts.net.", [4]byte{100, 64, 0, 1})}}
	h := NewHandler("tailnet", nil, nil)
	h.SetDNS(res)
	ctx := context.Background()

	t.Run("tailnet name keeps the client's ID and gets the answer", func(t *testing.T) {
		resp := h.answerDNS(ctx, buildQuery("echo.tail-scale.ts.net.", 0x1111))
		ips, rc := parseA(t, resp)
		if rc != dnsmessage.RCodeSuccess || len(ips) != 1 || ips[0].String() != "100.64.0.1" {
			t.Fatalf("got %v rcode %v", ips, rc)
		}
		if id := binary.BigEndian.Uint16(resp); id != 0x1111 {
			t.Errorf("response ID %#x, want the query's 0x1111", id)
		}
		if len(res.calls) != 1 || res.calls[0] != "A echo.tail-scale.ts.net." {
			t.Errorf("resolver calls: %v", res.calls)
		}
	})
	t.Run("name outside the tailnet is refused, never forwarded", func(t *testing.T) {
		resp := h.answerDNS(ctx, buildQuery("example.com.", 0x2222))
		_, rc := parseA(t, resp)
		if rc != dnsmessage.RCodeRefused {
			t.Errorf("rcode %v, want REFUSED", rc)
		}
		if id := binary.BigEndian.Uint16(resp); id != 0x2222 {
			t.Errorf("response ID %#x", id)
		}
	})
	t.Run("resolver failure becomes SERVFAIL", func(t *testing.T) {
		h2 := NewHandler("tailnet", nil, nil)
		h2.SetDNS(&fakeResolver{err: errors.New("boom")})
		_, rc := parseA(t, h2.answerDNS(ctx, buildQuery("echo.tail-scale.ts.net.", 3)))
		if rc != dnsmessage.RCodeServerFailure {
			t.Errorf("rcode %v", rc)
		}
	})
	t.Run("garbage becomes FORMERR", func(t *testing.T) {
		resp := h.answerDNS(ctx, []byte{1, 2, 3})
		if len(resp) < 12 || resp[3]&0x0f != byte(dnsmessage.RCodeFormatError) {
			t.Errorf("response %x", resp)
		}
	})
	t.Run("unsupported record type is NOTIMP", func(t *testing.T) {
		n := dnsmessage.MustNewName("echo.tail-scale.ts.net.")
		b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 9})
		b.StartQuestions()
		b.Question(dnsmessage.Question{Name: n, Type: dnsmessage.TypeHINFO, Class: dnsmessage.ClassINET})
		q, _ := b.Finish()
		_, rc := parseA(t, h.answerDNS(ctx, q))
		if rc != dnsmessage.RCodeNotImplemented {
			t.Errorf("rcode %v", rc)
		}
	})
}
