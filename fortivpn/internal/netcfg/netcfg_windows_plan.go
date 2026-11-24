package netcfg

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"

	"tunnelkit/fortivpn"
)

const nrptRulePrefix = "TunnelKit-"

type windowsAdapterOps interface {
	lookupGatewayRoute(gateway netip.Addr, skipLUID uint64, skipExistingHostRoute bool) (Snapshot, error)
	disableIPv6(luid uint64)
	flushTunnelRoutes(luid uint64) error
	configureIPv4Interface(luid uint64, mtu int) error
	disableDNSRegistration(luid uint64) error
	setIPv4Address(luid uint64, local netip.Addr) error
	addTunnelRoutes(luid uint64, routes []netip.Prefix) error
	addHostRoute(gwLUID uint64, prefix netip.Prefix, nextHop netip.Addr) (bool, error)
	deleteHostRoute(gwLUID uint64, prefix netip.Prefix, nextHop netip.Addr) error
	applyDNS(ctx context.Context, luid uint64, plan DNSPlan, journal *Journal) error
	removeDNS(ctx context.Context, luid uint64) error
	clearNRPT(ctx context.Context) error
	flushAdapter(luid uint64)
}

type windowsApplied struct {
	mu      sync.Mutex
	luid    uint64
	ops     windowsAdapterOps
	journal *Journal
	desired Desired
	snap    Snapshot
}

func applyWindowsWithOps(ctx context.Context, d Desired, journalPath string, luid uint64, ops windowsAdapterOps) (Applied, error) {
	if ops == nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: windows adapter ops are required", errors.New("nil adapter ops"))
	}
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: load journal failed", err)
	}

	d = NormalizeDesired(d)
	var snap Snapshot
	if NeedsGatewayHostRoute(d.Routes, d.Gateway) {
		snap, err = ops.lookupGatewayRoute(d.Gateway, luid, false)
		if err != nil {
			return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: lookup gateway route failed", err)
		}
	}

	ap := &windowsApplied{
		luid:    luid,
		ops:     ops,
		journal: journal,
		desired: d,
		snap:    snap,
	}

	steps := Plan(d, snap)
	if err := ap.executeSteps(ctx, steps); err != nil {
		_ = ap.Close(context.Background())
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: apply windows network configuration failed", err)
	}
	return ap, nil
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

	for _, s := range steps {
		switch s.Kind {
		case StepConfigureInterface:
			hasIface = true
			localAddr = s.Local
			mtu = s.MTU

		case StepHostRoute:
			gwLUID := a.snap.GatewayLUID
			if gwLUID != 0 {
				entry := buildHostRouteEntry(s.Prefix.Addr(), s.NextHop, "", gwLUID)
				if err := recordAndApplyHostRoute(a.journal, entry, func() (bool, error) {
					return a.ops.addHostRoute(gwLUID, s.Prefix, s.NextHop)
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
		if err := a.ops.applyDNS(ctx, a.luid, dnsPlan, a.journal); err != nil {
			return err
		}
	} else {
		_ = a.ops.removeDNS(ctx, a.luid)
	}
	return nil
}

// configureAdapterAndRoutes flushes stale routes before assigning the IPv4 /32
// address and adds tunnel routes via AddRoute so the kernel's local /32 route is preserved.
func (a *windowsApplied) configureAdapterAndRoutes(local netip.Addr, mtu int, routes []netip.Prefix) error {
	a.ops.disableIPv6(a.luid)
	_ = a.ops.flushTunnelRoutes(a.luid)

	if err := a.ops.setIPv4Address(a.luid, local); err != nil {
		return err
	}
	if err := a.ops.configureIPv4Interface(a.luid, mtu); err != nil {
		return err
	}
	// Best-effort: prevent Windows DNS client from sending dynamic DNS registration
	// probes on the virtual tunnel adapter without blocking locked-down enterprise hosts.
	_ = a.ops.disableDNSRegistration(a.luid)
	if err := a.ops.addTunnelRoutes(a.luid, routes); err != nil {
		return err
	}
	return nil
}

func (a *windowsApplied) Update(ctx context.Context, d Desired) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	d = NormalizeDesired(d)
	newNeedHost := NeedsGatewayHostRoute(d.Routes, d.Gateway)
	var (
		targetSnap       Snapshot
		hasTargetHost    bool
		desiredHostEntry JournalEntry
	)
	if newNeedHost {
		targetSnap = a.snap
		// Re-probe the active physical uplink while ignoring the tunnel adapter and
		// our own prior /32 host route so uplink switches refresh nextHop/LUID even
		// when the VPN gateway IP stays unchanged.
		if lookedUp, err := a.ops.lookupGatewayRoute(d.Gateway, a.luid, true); err == nil && lookedUp.GatewayLUID != 0 {
			targetSnap = lookedUp
		}
		hasTargetHost = targetSnap.GatewayLUID != 0
		if hasTargetHost {
			desiredHostEntry = buildHostRouteEntry(d.Gateway, targetSnap.GatewayNextHop, "", targetSnap.GatewayLUID)
		}
	}

	alreadyRecorded := false
	for _, entry := range a.journal.Entries() {
		if entry.Kind != KindHostRoute || entry.LUID == 0 || entry.Gateway == "" {
			continue
		}
		if newNeedHost && hasTargetHost && entry == desiredHostEntry {
			alreadyRecorded = true
			continue
		}
		if err := deleteWindowsHostRouteEntry(a.ops, entry); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous host route failed", err)
		}
		if err := a.journal.Remove(entry); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous host route journal failed", err)
		}
	}
	if newNeedHost && hasTargetHost && !alreadyRecorded {
		if err := recordAndApplyHostRoute(a.journal, desiredHostEntry, func() (bool, error) {
			return a.ops.addHostRoute(targetSnap.GatewayLUID, netip.PrefixFrom(d.Gateway, 32), targetSnap.GatewayNextHop)
		}); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update host route failed", err)
		}
	}
	if newNeedHost && hasTargetHost {
		a.snap = targetSnap
	}

	if err := a.configureAdapterAndRoutes(d.Local, d.MTU, d.Routes); err != nil {
		return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update interface and routes failed", err)
	}

	if HasEffectiveDNS(d.DNS) {
		if err := a.ops.applyDNS(ctx, a.luid, d.DNS, a.journal); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
	} else {
		if err := a.ops.removeDNS(ctx, a.luid); err != nil {
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

// EnsureDNS re-applies Wintun DNS/NRPT configuration and flushes the Windows
// DNS resolver cache after sleep/wake or network transitions.
func (a *windowsApplied) EnsureDNS(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !HasEffectiveDNS(a.desired.DNS) {
		return nil
	}
	return a.ops.applyDNS(ctx, a.luid, a.desired.DNS, a.journal)
}

func (a *windowsApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ops.flushAdapter(a.luid)
	return rollbackWindowsJournalWithOps(ctx, a.journal, a.ops)
}

func rollbackWindowsJournalWithOps(ctx context.Context, journal *Journal, ops windowsAdapterOps) error {
	return journal.Rollback(func(entry JournalEntry) error {
		switch entry.Kind {
		case KindWindowsNRPT:
			return ops.clearNRPT(ctx)
		case KindHostRoute:
			return deleteWindowsHostRouteEntry(ops, entry)
		default:
			return nil
		}
	})
}

func deleteWindowsHostRouteEntry(ops windowsAdapterOps, entry JournalEntry) error {
	if entry.LUID == 0 || entry.Gateway == "" {
		return nil
	}
	gwAddr, err := netip.ParseAddr(strings.TrimSuffix(entry.Gateway, "/32"))
	if err != nil {
		return nil
	}
	nextHop := netip.IPv4Unspecified()
	if entry.NextHop != "" {
		if nh, err := netip.ParseAddr(entry.NextHop); err == nil {
			nextHop = nh
		}
	}
	return ops.deleteHostRoute(entry.LUID, netip.PrefixFrom(gwAddr, 32), nextHop)
}
