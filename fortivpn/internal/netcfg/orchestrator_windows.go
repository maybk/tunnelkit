//go:build windows

package netcfg

import (
	"context"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"tunnelkit/fortivpn"
)

type windowsApplied struct {
	mu      sync.Mutex
	luid    winipcfg.LUID
	ops     windowsAdapterOps
	journal *Journal
	desired Desired
	snap    Snapshot
}

func (a *windowsApplied) adapterOps() windowsAdapterOps {
	if a.ops != nil {
		return a.ops
	}
	return realWindowsAdapterOps{}
}

func (a *windowsApplied) executeSteps(ctx context.Context, steps []Step) error {
	var (
		hasIface     bool
		localAddr    netip.Addr
		mtu          int
		tunnelRoutes []netip.Prefix
		hasDNS       bool
		dnsPlan      DNSPlan
	)
	ops := a.adapterOps()

	for _, s := range steps {
		switch s.Kind {
		case StepConfigureInterface:
			hasIface = true
			localAddr = s.Local
			mtu = s.MTU

		case StepHostRoute:
			gwLUID := winipcfg.LUID(a.snap.GatewayLUID)
			if gwLUID != 0 {
				entry := buildHostRouteEntry(s.Prefix.Addr(), s.NextHop, "", a.snap.GatewayLUID)
				if err := recordAndApplyHostRoute(a.journal, entry, func() (bool, error) {
					return ops.addHostRoute(gwLUID, s.Prefix, s.NextHop)
				}); err != nil {
					return err
				}
			}

		case StepTunnelRoute:
			tunnelRoutes = append(tunnelRoutes, s.Prefix)

		case StepDNS:
			hasDNS = true
			dnsPlan = s.DNS
		}
	}

	if hasIface {
		if err := a.configureAdapterAndRoutes(localAddr, mtu, tunnelRoutes); err != nil {
			return err
		}
	}

	if hasDNS {
		if err := ops.applyDNS(ctx, a.luid, dnsPlan, a.journal); err != nil {
			return err
		}
	} else {
		_ = ops.removeDNS(ctx, a.luid)
	}
	return nil
}

// configureAdapterAndRoutes flushes stale routes before assigning the IPv4 /32
// address and adds tunnel routes via AddRoute so the kernel's local /32 route is preserved.
func (a *windowsApplied) configureAdapterAndRoutes(local netip.Addr, mtu int, routes []netip.Prefix) error {
	ops := a.adapterOps()
	ops.disableIPv6(a.luid)
	_ = ops.flushTunnelRoutes(a.luid)

	if err := ops.setIPv4Address(a.luid, local); err != nil {
		return err
	}
	if err := ops.configureIPv4Interface(a.luid, mtu); err != nil {
		return err
	}
	if err := ops.addTunnelRoutes(a.luid, routes); err != nil {
		return err
	}
	return nil
}

func (a *windowsApplied) Update(ctx context.Context, d Desired) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	d = NormalizeDesired(d)
	ops := a.adapterOps()
	oldNeedHost := NeedsGatewayHostRoute(a.desired.Routes, a.desired.Gateway)
	newNeedHost := NeedsGatewayHostRoute(d.Routes, d.Gateway)
	for _, entry := range a.journal.Entries() {
		if entry.Kind != KindHostRoute || entry.LUID == 0 || entry.Gateway == "" || (newNeedHost && entry.Gateway == d.Gateway.String()) {
			continue
		}
		if gwAddr, err := netip.ParseAddr(entry.Gateway); err == nil {
			nextHop := netip.IPv4Unspecified()
			if entry.NextHop != "" {
				if nh, err := netip.ParseAddr(entry.NextHop); err == nil {
					nextHop = nh
				}
			}
			if err := ops.deleteHostRoute(winipcfg.LUID(entry.LUID), netip.PrefixFrom(gwAddr, 32), nextHop); err != nil {
				return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous host route failed", err)
			}
		}
		if err := a.journal.Remove(entry); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous host route journal failed", err)
		}
	}
	if newNeedHost && (!oldNeedHost || d.Gateway != a.desired.Gateway) {
		snap := a.snap
		if snap.GatewayLUID == 0 || d.Gateway != a.desired.Gateway {
			if lookedUp, err := ops.lookupGatewayRoute(d.Gateway, a.luid); err == nil && lookedUp.GatewayLUID != 0 {
				snap = lookedUp
				a.snap = snap
			}
		}
		gwLUID := winipcfg.LUID(snap.GatewayLUID)
		if gwLUID != 0 {
			entry := buildHostRouteEntry(d.Gateway, snap.GatewayNextHop, "", snap.GatewayLUID)
			if err := recordAndApplyHostRoute(a.journal, entry, func() (bool, error) {
				return ops.addHostRoute(gwLUID, netip.PrefixFrom(d.Gateway, 32), snap.GatewayNextHop)
			}); err != nil {
				return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update host route failed", err)
			}
		}
	}

	if err := a.configureAdapterAndRoutes(d.Local, d.MTU, d.Routes); err != nil {
		return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update interface and routes failed", err)
	}

	if HasEffectiveDNS(d.DNS) {
		if err := ops.applyDNS(ctx, a.luid, d.DNS, a.journal); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
	} else {
		if err := ops.removeDNS(ctx, a.luid); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove dns failed", err)
		}
		if err := a.journal.Remove(JournalEntry{
			Kind: KindWindowsNRPT,
			Key:  nrptRulePrefix,
		}); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove dns journal failed", err)
		}
	}

	a.desired = d
	return nil
}

func (a *windowsApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.adapterOps().flushAdapter(a.luid)
	return rollbackWindowsJournalWithOps(ctx, a.journal, a.adapterOps())
}

func rollbackWindowsJournal(ctx context.Context, journal *Journal) error {
	return rollbackWindowsJournalWithOps(ctx, journal, realWindowsAdapterOps{})
}

func rollbackWindowsJournalWithOps(ctx context.Context, journal *Journal, ops windowsAdapterOps) error {
	return journal.Rollback(func(entry JournalEntry) error {
		switch entry.Kind {
		case KindWindowsNRPT:
			err := clearWindowsNRPT()
			flushWindowsDNSCache(ctx)
			return err
		case KindHostRoute:
			if entry.LUID != 0 && entry.Gateway != "" {
				gwAddr, err := netip.ParseAddr(entry.Gateway)
				if err == nil {
					nextHop := netip.IPv4Unspecified()
					if entry.NextHop != "" {
						if nh, err := netip.ParseAddr(entry.NextHop); err == nil {
							nextHop = nh
						}
					}
					luid := winipcfg.LUID(entry.LUID)
					return ops.deleteHostRoute(luid, netip.PrefixFrom(gwAddr, 32), nextHop)
				}
			}
			return nil
		default:
			return nil
		}
	})
}
