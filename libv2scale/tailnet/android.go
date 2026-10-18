//go:build android

package tailnet

import (
	"net"
	"net/netip"
	"time"

	"tailscale.com/net/netmon"
)

// Baseline for Android: Phase 3 replaces this with the interface list from Java.
//
// Android 11+ denies app UIDs netlink and /proc/net, so net.Interfaces fails
// and netmon cannot start. Report one synthetic interface whose addresses are
// the sources the kernel picks for outbound UDP sockets (no packets are sent).
// This mirrors tailscale.com/feature/androidbin, which is not registered in
// cgo builds such as gomobile's.
func init() {
	netmon.RegisterInterfaceGetter(syntheticInterfaces)
}

func syntheticInterfaces() ([]netmon.Interface, error) {
	var addrs []net.Addr
	if ip, ok := outboundIP("udp4", "8.8.8.8:53"); ok {
		addrs = append(addrs, &net.IPNet{IP: ip.AsSlice(), Mask: net.CIDRMask(32, 32)})
	}
	if ip, ok := outboundIP("udp6", "[2001:4860:4860::8888]:53"); ok {
		addrs = append(addrs, &net.IPNet{IP: ip.AsSlice(), Mask: net.CIDRMask(128, 128)})
	}
	return []netmon.Interface{{
		Interface: &net.Interface{
			MTU:   1500,
			Name:  "android",
			Flags: net.FlagUp | net.FlagRunning,
		},
		AltAddrs: addrs,
	}}, nil
}

func outboundIP(network, addr string) (netip.Addr, bool) {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.Dial(network, addr)
	if err != nil {
		return netip.Addr{}, false
	}
	defer c.Close()
	ua, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok {
		return netip.Addr{}, false
	}
	ip, ok := netip.AddrFromSlice(ua.IP)
	if !ok {
		return netip.Addr{}, false
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsUnspecified() {
		return netip.Addr{}, false
	}
	return ip, true
}
