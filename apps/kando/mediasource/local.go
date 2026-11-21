package mediasource

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// Resolver looks a host name up, as net.Resolver.LookupNetIP does.
type Resolver func(ctx context.Context, host string) ([]netip.Addr, error)

// Local reports whether host, a name or an address, is on the local network, which offline mode leaves open: every address it resolves to is a loopback, private (RFC 1918, fc00::/7) or link-local one. A name is judged by what it resolves to, not by how it reads, so a .local name that resolves to a public address is not local; and the shared address space of carrier-grade NAT, 100.64.0.0/10, where Tailscale's addresses are, counts as the internet. An error is a name that could not be resolved, which is not local either.
func Local(ctx context.Context, host string) (bool, error) {
	return LocalBy(ctx, host, func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	})
}

// LocalBy is Local resolving a name with resolve.
func LocalBy(ctx context.Context, host string, resolve Resolver) (bool, error) {
	host = strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"), ".")
	if host == "" {
		return false, errors.New("mediasource: no host")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return localAddr(addr), nil
	}
	addrs, err := resolve(ctx, host)
	if err != nil {
		return false, err
	}
	if len(addrs) == 0 {
		return false, fmt.Errorf("mediasource: %s resolves to no address", host)
	}
	for _, addr := range addrs {
		if !localAddr(addr) {
			return false, nil
		}
	}
	return true, nil
}

// localAddr reports whether addr is a loopback, private or link-local unicast address, an IPv4 one written as IPv6 alike.
func localAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}
