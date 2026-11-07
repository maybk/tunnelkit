package engine

import (
	"context"
	"net/netip"
	"slices"
	"strings"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netcfg"
	"tunnelkit/fortivpn/internal/sslvpn"
)

func (s *session) applyOrUpdateNetcfg(ctx context.Context, desired netcfg.Desired) error {
	if s.opts.device != nil {
		// Injected device mode (e.g. fortitest.MemDevice): do not touch host routes/DNS.
		return nil
	}

	s.mu.Lock()
	applied := s.applied
	s.mu.Unlock()

	if applied == nil {
		s.log.Info("Setting new routes...")
		if desired.DNS.Mode != fortivpn.DNSOff && len(desired.DNS.Servers) > 0 && !netcfg.SplitDNSNeedsDomains(desired.DNS) {
			s.log.Info("Adding VPN nameservers...")
		}
		ap, err := netcfg.Apply(ctx, desired, s.opts.journalPath)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.applied = ap
		s.mu.Unlock()
		return nil
	}

	return applied.Update(ctx, desired)
}

func (s *session) buildDesiredAndInfo(
	cfg fortivpn.Config,
	gatewayIP netip.Addr,
	tunCfg sslvpn.TunnelConfig,
	neg negotiatedPPP,
	ifaceName string,
) (netcfg.Desired, fortivpn.TunnelInfo, error) {
	localIP := neg.ipCfg.LocalIP
	if !localIP.IsValid() || localIP.IsUnspecified() {
		localIP = tunCfg.AssignedIP
	}
	peerIP := neg.ipCfg.PeerIP
	if !peerIP.IsValid() || peerIP.IsUnspecified() || peerIP == gatewayIP || peerIP == localIP {
		peerIP = netip.MustParseAddr("169.254.2.1")
	}

	excludes := append([]netip.Prefix(nil), tunCfg.ExcludeRoutes...)

	routes, _, err := netcfg.SanitizeRoutes(
		cfg.Routes,
		tunCfg.SplitRoutes,
		cfg.ExtraRoutes,
		excludes,
		gatewayIP,
	)
	if err != nil {
		return netcfg.Desired{}, fortivpn.TunnelInfo{}, err
	}

	dnsServers := mergeAddrs(neg.ipCfg.DNS, tunCfg.DNS)
	dnsDomains := mergeDomains(tunCfg.DNSDomains, cfg.DNSDomains)

	dnsPlan := netcfg.DNSPlan{
		Mode:    cfg.DNS,
		Servers: dnsServers,
		Domains: dnsDomains,
	}
	splitNeedsDomains := netcfg.SplitDNSNeedsDomains(dnsPlan)

	desired := netcfg.Desired{
		Iface:    ifaceName,
		Local:    localIP,
		Peer:     peerIP,
		MTU:      neg.mru,
		Routes:   routes,
		Excludes: excludes,
		Gateway:  gatewayIP,
		DNS:      dnsPlan,
	}

	routeStrs := make([]string, len(routes))
	for i, r := range routes {
		routeStrs[i] = r.String()
	}
	dnsServerStrs := make([]string, len(dnsServers))
	for i, a := range dnsServers {
		dnsServerStrs[i] = a.String()
	}

	var gwIPStr string
	if gatewayIP.IsValid() {
		gwIPStr = gatewayIP.String()
	}

	info := fortivpn.TunnelInfo{
		Interface:            ifaceName,
		LocalIP:              localIP.String(),
		PeerIP:               peerIP.String(),
		MTU:                  neg.mru,
		RouteCount:           len(routes),
		Routes:               routeStrs,
		DNSMode:              cfg.DNS,
		DNSServers:           dnsServerStrs,
		DNSDomains:           dnsDomains,
		SplitDNSNeedsDomains: splitNeedsDomains,
		GatewayHost:          cfg.Host,
		GatewayIP:            gwIPStr,
		FortiOS:              tunCfg.FortiOS,
	}
	return desired, info, nil
}

func mergeAddrs(primary, secondary []netip.Addr) []netip.Addr {
	var out []netip.Addr
	for _, list := range [][]netip.Addr{primary, secondary} {
		for _, a := range list {
			if !a.IsValid() || a.IsUnspecified() {
				continue
			}
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return out
}

func mergeDomains(primary, secondary []string) []string {
	var out []string
	for _, list := range [][]string{primary, secondary} {
		for _, d := range list {
			cleaned := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(d, ".")))
			if cleaned != "" && !slices.Contains(out, cleaned) {
				out = append(out, cleaned)
			}
		}
	}
	return out
}
