package netcfg

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"

	"tunnelkit/fortivpn"
)

const (
	// DefaultMTU is the fallback tunnel MTU when none is specified.
	DefaultMTU = 1400
)

var (
	// DefaultPeerIP is the synthetic point-to-point peer address used when none is negotiated.
	DefaultPeerIP   = netip.MustParseAddr("169.254.2.1")
	halfDefaultLow  = netip.MustParsePrefix("0.0.0.0/1")
	halfDefaultHigh = netip.MustParsePrefix("128.0.0.0/1")
)

// NormalizeDesired returns a copy of d with MTU and Peer normalized to safe defaults.
func NormalizeDesired(d Desired) Desired {
	if d.MTU <= 0 {
		d.MTU = DefaultMTU
	}
	if !d.Peer.IsValid() || d.Peer.IsUnspecified() || d.Peer == d.Gateway || d.Peer == d.Local {
		d.Peer = DefaultPeerIP
	}
	return d
}

// HasEffectiveDNS reports whether plan enables DNS and contains at least one valid,
// non-unspecified DNS server (and has domains when in split-DNS mode).
func HasEffectiveDNS(plan DNSPlan) bool {
	if plan.Mode == fortivpn.DNSOff || SplitDNSNeedsDomains(plan) {
		return false
	}
	for _, s := range plan.Servers {
		if s.IsValid() && !s.IsUnspecified() {
			return true
		}
	}
	return false
}

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

// DiffRoutes partitions oldRoutes and newRoutes into routes to remove, routes
// already active to keep, and new routes to add, preserving input order.
func DiffRoutes(oldRoutes, newRoutes []netip.Prefix) (toRemove, toKeep, toAdd []netip.Prefix) {
	oldSet := make(map[netip.Prefix]struct{}, len(oldRoutes))
	for _, r := range oldRoutes {
		oldSet[r] = struct{}{}
	}
	newSet := make(map[netip.Prefix]struct{}, len(newRoutes))
	for _, r := range newRoutes {
		newSet[r] = struct{}{}
	}
	for _, r := range oldRoutes {
		if _, keep := newSet[r]; keep {
			toKeep = append(toKeep, r)
		} else {
			toRemove = append(toRemove, r)
		}
	}
	for _, r := range newRoutes {
		if _, exists := oldSet[r]; !exists {
			toAdd = append(toAdd, r)
		}
	}
	return toRemove, toKeep, toAdd
}

// Plan computes the ordered sequence of network configuration steps from Desired and Snapshot.
func Plan(d Desired, snap Snapshot) []Step {
	var steps []Step
	norm := NormalizeDesired(d)

	steps = append(steps, Step{
		Kind:  StepConfigureInterface,
		Iface: norm.Iface,
		Local: norm.Local,
		Peer:  norm.Peer,
		MTU:   norm.MTU,
	})

	if NeedsGatewayHostRoute(norm.Routes, norm.Gateway) && (snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" || snap.GatewayLUID != 0) {
		steps = append(steps, Step{
			Kind:    StepHostRoute,
			Iface:   snap.GatewayIface,
			Prefix:  netip.PrefixFrom(norm.Gateway, 32),
			NextHop: snap.GatewayNextHop,
		})
	}

	for _, r := range norm.Routes {
		steps = append(steps, Step{
			Kind:   StepTunnelRoute,
			Iface:  norm.Iface,
			Prefix: r,
		})
	}

	if HasEffectiveDNS(norm.DNS) {
		steps = append(steps, Step{
			Kind:  StepDNS,
			Iface: norm.Iface,
			DNS:   norm.DNS,
		})
	}

	return steps
}
