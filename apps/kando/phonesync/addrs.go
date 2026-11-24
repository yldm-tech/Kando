package phonesync

import (
	"net"
	"net/netip"
	"slices"
	"strings"
)

// cgnat is 100.64.0.0/10, the shared address space carriers and Tailscale give out, which is no local network a phone beside the computer is on.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// candidate is an IPv4 address of one of the computer's network interfaces, and whether that interface looks like a VPN's.
type candidate struct {
	addr netip.Addr
	vpn  bool
}

// LANAddrs are the computer's addresses on its local networks, which the server listens on and the pairing code lists: the private IPv4 addresses (10/8, 172.16/12, 192.168/16) of the interfaces that are up, loopback left out. Those of a VPN or Tailscale (100.64.0.0/10, a point-to-point link, an interface named for a VPN) are used only when there are no others.
func LANAddrs() []netip.Addr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var found []candidate
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		vpn := ifi.Flags&net.FlagPointToPoint != 0 || vpnName(ifi.Name)
		for _, a := range addrs {
			prefix, err := netip.ParsePrefix(a.String())
			if err != nil {
				continue
			}
			found = append(found, candidate{addr: prefix.Addr().Unmap(), vpn: vpn})
		}
	}
	return pickAddrs(found)
}

// pickAddrs keeps the IPv4 addresses of found that are on a local network, in order and once each: those of no VPN when there are any, else those of a VPN or Tailscale.
func pickAddrs(found []candidate) []netip.Addr {
	var lan, vpn []netip.Addr
	for _, c := range found {
		a := c.addr.Unmap()
		if !a.Is4() || a.IsLoopback() || !(a.IsPrivate() || cgnat.Contains(a)) {
			continue
		}
		if c.vpn || cgnat.Contains(a) {
			if !slices.Contains(vpn, a) {
				vpn = append(vpn, a)
			}
			continue
		}
		if !slices.Contains(lan, a) {
			lan = append(lan, a)
		}
	}
	if len(lan) > 0 {
		return lan
	}
	return vpn
}

// vpnName reports whether an interface's name is a VPN's: utun, tun, tap, wg, ppp, ipsec and zt on Unix, feth for ZeroTier on macOS, and on Windows, where interfaces have names people gave them, one that says VPN, Tailscale, WireGuard or ZeroTier.
func vpnName(name string) bool {
	n := strings.ToLower(name)
	for _, prefix := range []string{"utun", "tun", "tap", "wg", "ppp", "ipsec", "zt", "feth"} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	for _, word := range []string{"vpn", "tailscale", "wireguard", "zerotier"} {
		if strings.Contains(n, word) {
			return true
		}
	}
	return false
}
