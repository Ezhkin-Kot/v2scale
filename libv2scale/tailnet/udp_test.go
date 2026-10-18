package tailnet

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/xtls/xray-core/features/stats"
	"golang.org/x/net/dns/dnsmessage"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

func serveUDPEcho(t *testing.T, s *tsnet.Server, ip netip.Addr, port string) {
	t.Helper()
	pc, err := s.ListenPacket("udp", net.JoinHostPort(ip.String(), port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(buf[:n], addr)
		}
	}()
}

const statsConfig = `"stats": {},
  "policy": {"system": {"statsOutboundUplink": true, "statsOutboundDownlink": true}},`

// udpForwarder builds an Xray instance whose dokodemo inbound forwards UDP to target
// (the way the tun inbound hands application flows to routing), and returns its address.
func udpForwarder(t *testing.T, ctx context.Context, phone *Node, target string) (addr string, counters func(string) int64) {
	t.Helper()
	host, port, _ := net.SplitHostPort(target)
	p := freePort(t)
	cfg := fmt.Sprintf(`{
	  "log": {"loglevel": "none"},
	  %s
	  "inbounds": [{"tag":"udp-in","listen":"127.0.0.1","port":%d,"protocol":"dokodemo-door",
	      "settings":{"address":%q,"port":%s,"network":"udp"}}],
	  "outbounds": [%s],
	  "routing": {"domainStrategy":"AsIs","rules":[%s]}
	}`, statsConfig, p, host, port, baseOutbounds, tsRules)
	inst, err := buildInstance(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Attach(ctx, inst, DefaultTag, DialFunc(phone.DialContext)); err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}
	sm := inst.GetFeature(stats.ManagerType()).(stats.Manager)
	return fmt.Sprintf("127.0.0.1:%d", p), func(name string) int64 {
		if c := sm.GetCounter(name); c != nil {
			return c.Value()
		}
		return -1
	}
}

// UDP datagrams cross the tailnet outbound with their boundaries intact, and the traffic
// counters see them.
func TestUDPThroughTailnet(t *testing.T) {
	controlURL, _ := startControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	server, serverIP := startNode(t, ctx, controlURL, "echo")
	serveUDPEcho(t, server, serverIP, "9")
	phone := startTestNode(t, ctx, controlURL, "phone", nil)

	addr, counter := udpForwarder(t, ctx, phone, net.JoinHostPort(serverIP.String(), "9"))
	c, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	total := 0
	for _, size := range []int{1, 17, 512, 1200} {
		msg := make([]byte, size)
		for i := range msg {
			msg[i] = byte(size + i)
		}
		c.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write(msg); err != nil {
			t.Fatal(err)
		}
		back := make([]byte, 2048)
		n, err := c.Read(back)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if n != size || string(back[:n]) != string(msg) {
			t.Fatalf("size %d: got %d bytes (datagram boundary or content lost)", size, n)
		}
		total += size
	}
	if up := counter("outbound>>>tailnet>>>traffic>>>uplink"); up < int64(total) {
		t.Errorf("uplink counter = %d, want >= %d", up, total)
	}
	if down := counter("outbound>>>tailnet>>>traffic>>>downlink"); down < int64(total) {
		t.Errorf("downlink counter = %d, want >= %d", down, total)
	}
}

func buildQuery(name string, id uint16) []byte {
	n, _ := dnsmessage.NewName(name)
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	b.EnableCompression()
	b.StartQuestions()
	b.Question(dnsmessage.Question{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET})
	out, _ := b.Finish()
	return out
}

func parseA(t *testing.T, msg []byte) ([]netip.Addr, dnsmessage.RCode) {
	t.Helper()
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	if err != nil {
		t.Fatal(err)
	}
	p.SkipAllQuestions()
	var out []netip.Addr
	for {
		rh, err := p.AnswerHeader()
		if err != nil {
			break
		}
		if rh.Type == dnsmessage.TypeA {
			r, _ := p.AResource()
			out = append(out, netip.AddrFrom4(r.A))
		} else {
			p.SkipAnswer()
		}
	}
	return out, h.RCode
}

// Experiment 2.2: Xray's DNS module can use the node's MagicDNS (100.100.100.100) as an
// upstream, over UDP and over TCP. Routing sends that address into the tailnet outbound,
// whose DNS bridge answers from the node's resolver. Queries arrive through a dokodemo
// inbound and a dns outbound, the way the tun inbound hands them over on a phone.
func TestMagicDNSUpstreamThroughXray(t *testing.T) {
	for _, upstream := range []string{"100.100.100.100", "tcp://100.100.100.100"} {
		t.Run(upstream, func(t *testing.T) { testMagicDNS(t, upstream) })
	}
}

func testMagicDNS(t *testing.T, upstream string) {
	controlURL, control := startControl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, serverIP := startNode(t, ctx, controlURL, "echo")
	control.AddDNSRecords(
		tailcfg.DNSRecord{Name: "svc.example.test", Type: "A", Value: serverIP.String()},
		tailcfg.DNSRecord{Name: "svc.tail-scale.ts.net", Type: "A", Value: serverIP.String()})
	phone := startTestNode(t, ctx, controlURL, "phone", nil)

	p := freePort(t)
	cfg := fmt.Sprintf(`{
	  "log": {"loglevel": "none"},
	  "dns": {"servers": [{"address":%q,"port":53}], "queryStrategy":"UseIPv4", "disableCache": true},
	  "inbounds": [{"tag":"dns-in","listen":"127.0.0.1","port":%d,"protocol":"dokodemo-door",
	      "settings":{"address":"1.1.1.1","port":53,"network":"udp"}}],
	  "outbounds": [%s, {"protocol":"dns","tag":"dns-out"}],
	  "routing": {"domainStrategy":"AsIs","rules":[
	      {"type":"field","inboundTag":["dns-in"],"outboundTag":"dns-out"},
	      %s]}
	}`, upstream, p, baseOutbounds, tsRules)
	inst, err := buildInstance(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AttachNode(ctx, inst, DefaultTag, phone); err != nil {
		t.Fatal(err)
	}
	if err := inst.Start(); err != nil {
		t.Fatal(err)
	}

	ask := func(name string) ([]netip.Addr, dnsmessage.RCode) {
		c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(8 * time.Second))
		if _, err := c.Write(buildQuery(name, 0x1234)); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 1500)
		n, err := c.Read(buf)
		if err != nil {
			t.Fatalf("no DNS answer for %s: %v", name, err)
		}
		return parseA(t, buf[:n])
	}

	for name, want := range map[string]string{
		"echo.tail-scale.ts.net.": "MagicDNS peer name",
		"svc.tail-scale.ts.net.":  "extra record of type A inside the tailnet domain",
		"svc.example.test.":       "extra record of type A outside the tailnet domain",
	} {
		ips, rc := ask(name)
		if len(ips) != 1 || ips[0] != serverIP {
			t.Errorf("%s (%s): got %v (rcode %v), want %v", name, want, ips, rc, serverIP)
		}
	}
}
