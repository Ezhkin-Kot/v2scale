// Command tailnet-probe brings up the embedded tailnet node against a real control server
// (Headscale or Tailscale) and runs a few checks. It is a development tool for smoke tests
// outside the hermetic test environment.
//
//	TS_AUTHKEY=... tailnet-probe -control http://127.0.0.1:7080 -state ./state \
//	    -hostname v2scale-test -dial ezhkin-drive:443 -resolve nextcloud.example.net
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"v2scale/libv2scale/tailnet"
)

func main() {
	control := flag.String("control", "", "control server URL")
	state := flag.String("state", "./state", "state directory (keeps the node identity)")
	host := flag.String("hostname", "v2scale-test", "node name")
	proxyURL := flag.String("control-proxy", "", "HTTP proxy for the control connection")
	dial := flag.String("dial", "", "host:port to connect to through the tailnet and TLS-handshake with")
	resolve := flag.String("resolve", "", "comma-separated names to resolve through the node's resolver")
	wait := flag.Duration("wait", 90*time.Second, "how long to wait for the node to come up")
	flag.Parse()

	cfg := tailnet.Config{
		ControlURL:   *control,
		AuthKey:      os.Getenv("TS_AUTHKEY"),
		Hostname:     *host,
		StateDir:     *state,
		ControlProxy: *proxyURL,
	}
	n, err := tailnet.NewNode(cfg)
	if err != nil {
		fatal(err)
	}
	defer n.Close()
	if err := n.Start(); err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *wait)
	defer cancel()
	if err := n.WaitUp(ctx); err != nil {
		st := n.Status(context.Background())
		fatal(fmt.Errorf("node did not come up: %v (status %+v)", err, st))
	}
	st := n.Status(ctx)
	fmt.Printf("node %q is %s, addresses %v\n", *host, st.State, st.IPs)

	for _, name := range strings.Split(*resolve, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		raw, err := n.QueryDNS(ctx, name, "A")
		if err != nil {
			fmt.Printf("resolve %s: %v\n", name, err)
			continue
		}
		var p dnsmessage.Parser
		h, _ := p.Start(raw)
		p.SkipAllQuestions()
		var ips []string
		for {
			rh, err := p.AnswerHeader()
			if err != nil {
				break
			}
			if rh.Type == dnsmessage.TypeA {
				r, _ := p.AResource()
				ips = append(ips, fmt.Sprintf("%d.%d.%d.%d", r.A[0], r.A[1], r.A[2], r.A[3]))
			} else {
				p.SkipAnswer()
			}
		}
		fmt.Printf("resolve %s: %v (rcode %v)\n", name, ips, h.RCode)
	}

	if *dial != "" {
		start := time.Now()
		c, err := n.DialContext(ctx, "tcp", *dial)
		if err != nil {
			fmt.Printf("dial %s: FAILED after %v: %v\n", *dial, time.Since(start).Round(time.Millisecond), err)
			os.Exit(2)
		}
		defer c.Close()
		fmt.Printf("dial %s: connected in %v\n", *dial, time.Since(start).Round(time.Millisecond))
		hostOnly := *dial
		if i := strings.LastIndex(hostOnly, ":"); i > 0 {
			hostOnly = hostOnly[:i]
		}
		tc := tls.Client(c, &tls.Config{ServerName: hostOnly, InsecureSkipVerify: true})
		if err := tc.HandshakeContext(ctx); err != nil {
			fmt.Printf("TLS handshake: %v\n", err)
			os.Exit(2)
		}
		cs := tc.ConnectionState()
		fmt.Printf("TLS handshake ok (%x), peer certificate subject: %v\n", cs.Version, cs.PeerCertificates[0].Subject)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
