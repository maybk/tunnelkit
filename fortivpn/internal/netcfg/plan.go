package netcfg

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"

	"tunnelkit/fortivpn"
)

var (
	halfDefaultLow  = netip.MustParsePrefix("0.0.0.0/1")
	halfDefaultHigh = netip.MustParsePrefix("128.0.0.0/1")
)

// SanitizeRoutes deduplicates, filters, and sorts tunnel routes according to
// the configured RouteMode and safety rules.
func SanitizeRoutes(
	mode fortivpn.RouteMode,
	gatewayRoutes []netip.Prefix,
	extraRoutes []netip.Prefix,
	excludeRoutes []netip.Prefix,
	gatewayIP netip.Addr,
) ([]netip.Prefix, bool, error) {
	excluded := make(map[netip.Prefix]struct{}, len(excludeRoutes))
	for _, ex := range excludeRoutes {
		if ex.IsValid() && ex.Addr().Is4() {
			excluded[ex.Masked()] = struct{}{}
		}
	}

	var raw []netip.Prefix
	switch mode {
	case fortivpn.RouteNone:
		raw = append(raw, extraRoutes...)
	case fortivpn.RouteFull:
		raw = append(raw, halfDefaultLow, halfDefaultHigh)
		raw = append(raw, extraRoutes...)
	default: // RouteGateway
		if len(gatewayRoutes) == 0 {
			raw = append(raw, halfDefaultLow, halfDefaultHigh)
		} else {
			for _, p := range gatewayRoutes {
				if p.IsValid() && p.Addr().Is4() && p.Bits() == 0 {
					raw = append(raw, halfDefaultLow, halfDefaultHigh)
				} else {
					raw = append(raw, p)
				}
			}
		}
		raw = append(raw, extraRoutes...)
	}

	seen := make(map[netip.Prefix]struct{}, len(raw))
	var out []netip.Prefix
	for _, p := range raw {
		if !p.IsValid() || !p.Addr().Is4() {
			continue
		}
		masked := p.Masked()
		if masked.Bits() == 0 {
			continue
		}
		addr := masked.Addr()
		if addr.IsLoopback() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || (addr.IsUnspecified() && masked != halfDefaultLow) {
			continue
		}
		if gatewayIP.IsValid() && masked.Bits() == 32 && addr == gatewayIP {
			continue
		}
		if _, isEx := excluded[masked]; isEx {
			continue
		}
		if _, ok := seen[masked]; ok {
			continue
		}
		seen[masked] = struct{}{}
		out = append(out, masked)
	}

	slices.SortFunc(out, func(a, b netip.Prefix) int {
		if c := a.Addr().Compare(b.Addr()); c != 0 {
			return c
		}
		return cmp.Compare(a.Bits(), b.Bits())
	})

	if len(out) > MaxRoutes {
		return nil, false, fortivpn.NewError(
			fortivpn.CodeNetcfgFailed,
			fmt.Sprintf("route count %d exceeds maximum %d", len(out), MaxRoutes),
		)
	}
	return out, false, nil
}

// NeedsGatewayHostRoute reports whether any tunnel route covers the gateway's
// public IPv4 address (which requires pinning a /32 host route via the physical
// gateway before adding tunnel routes).
func NeedsGatewayHostRoute(routes []netip.Prefix, gatewayIP netip.Addr) bool {
	if !gatewayIP.IsValid() || !gatewayIP.Is4() || gatewayIP.IsLoopback() {
		return false
	}
	for _, p := range routes {
		if p.Contains(gatewayIP) {
			return true
		}
	}
	return false
}

// SplitDNSNeedsDomains reports whether split DNS was requested with DNS servers
// present, but neither the gateway nor the user supplied any DNS domains.
func SplitDNSNeedsDomains(dns DNSPlan) bool {
	return dns.Mode == fortivpn.DNSSplit && len(dns.Servers) > 0 && len(dns.Domains) == 0
}

// Plan computes the ordered sequence of network configuration steps from Desired and Snapshot.
func Plan(d Desired, snap Snapshot) []Step {
	var steps []Step

	mtu := d.MTU
	if mtu <= 0 {
		mtu = 1400
	}
	peer := d.Peer
	if !peer.IsValid() || peer.IsUnspecified() || peer == d.Gateway || peer == d.Local {
		peer = netip.MustParseAddr("169.254.2.1")
	}

	steps = append(steps, Step{
		Kind:  StepConfigureInterface,
		Iface: d.Iface,
		Local: d.Local,
		Peer:  peer,
		MTU:   mtu,
	})

	if NeedsGatewayHostRoute(d.Routes, d.Gateway) && (snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" || snap.GatewayLUID != 0) {
		steps = append(steps, Step{
			Kind:    StepHostRoute,
			Iface:   snap.GatewayIface,
			Prefix:  netip.PrefixFrom(d.Gateway, 32),
			NextHop: snap.GatewayNextHop,
		})
	}

	for _, r := range d.Routes {
		steps = append(steps, Step{
			Kind:   StepTunnelRoute,
			Iface:  d.Iface,
			Prefix: r,
		})
	}

	if d.DNS.Mode != fortivpn.DNSOff && len(d.DNS.Servers) > 0 && !SplitDNSNeedsDomains(d.DNS) {
		steps = append(steps, Step{
			Kind:  StepDNS,
			Iface: d.Iface,
			DNS:   d.DNS,
		})
	}

	return steps
}
