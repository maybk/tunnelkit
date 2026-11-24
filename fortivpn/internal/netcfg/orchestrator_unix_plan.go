package netcfg

import (
	"context"
	"net/netip"
	"slices"
	"sync"

	"tunnelkit/fortivpn"
)

type unixPlatformOps struct {
	name               string
	lookupRoute        func(ctx context.Context, run cmdRunner, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error)
	configureInterface func(ctx context.Context, run cmdRunner, prev Desired, next Desired) error
	addHostRoute       func(ctx context.Context, run cmdRunner, host netip.Addr, nextHop netip.Addr, iface string) (bool, error)
	deleteHostRoute    func(ctx context.Context, run cmdRunner, host string, nextHop string, iface string) error
	addTunnelRoute     func(ctx context.Context, run cmdRunner, prefix netip.Prefix, iface string) (bool, error)
	deleteTunnelRoute  func(ctx context.Context, run cmdRunner, prefix netip.Prefix, iface string) error
	applyDNS           func(ctx context.Context, run cmdRunner, iface string, plan DNSPlan, journal *Journal) (string, error)
	removeDNS          func(ctx context.Context, run cmdRunner, iface string, key string) error
	ensureDNS          func(ctx context.Context, run cmdRunner, iface string, key string, plan DNSPlan, journal *Journal) (string, error)
	closeOps           func() error
	dnsJournalKind     JournalEntryKind
	dnsStaticKey       string
}

type unixApplied struct {
	mu           sync.Mutex
	run          cmdRunner
	ops          unixPlatformOps
	journal      *Journal
	desired      Desired
	snap         Snapshot
	tunnelRoutes []netip.Prefix
	dnsKey       string
}

func applyUnixWithOps(ctx context.Context, d Desired, journalPath string, run cmdRunner, ops unixPlatformOps) (Applied, error) {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		if ops.closeOps != nil {
			_ = ops.closeOps()
		}
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: load journal failed", err)
	}

	d = NormalizeDesired(d)
	var snap Snapshot
	if NeedsGatewayHostRoute(d.Routes, d.Gateway) {
		snap, err = ops.lookupRoute(ctx, run, d.Gateway, d.Iface, false)
		if err != nil {
			if ops.closeOps != nil {
				_ = ops.closeOps()
			}
			return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: lookup gateway route failed", err)
		}
	}

	steps := Plan(d, snap)
	ap := &unixApplied{
		run:     run,
		ops:     ops,
		journal: journal,
		desired: d,
		snap:    snap,
		dnsKey:  ops.dnsStaticKey,
	}

	if err := ap.executeSteps(ctx, steps); err != nil {
		_ = ap.Close(context.Background())
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: apply "+ops.name+" network configuration failed", err)
	}
	return ap, nil
}

func (a *unixApplied) executeSteps(ctx context.Context, steps []Step) error {
	for _, s := range steps {
		switch s.Kind {
		case StepConfigureInterface:
			next := Desired{Iface: s.Iface, Local: s.Local, Peer: s.Peer, MTU: s.MTU}
			if err := a.ops.configureInterface(ctx, a.run, Desired{}, next); err != nil {
				return err
			}

		case StepHostRoute:
			entry := buildHostRouteEntry(s.Prefix.Addr(), s.NextHop, s.Iface, 0)
			if err := recordAndApplyHostRoute(a.journal, entry, func() (bool, error) {
				return a.ops.addHostRoute(ctx, a.run, s.Prefix.Addr(), s.NextHop, s.Iface)
			}); err != nil {
				return err
			}

		case StepTunnelRoute:
			created, err := a.ops.addTunnelRoute(ctx, a.run, s.Prefix, s.Iface)
			if err != nil {
				return err
			}
			if created {
				a.tunnelRoutes = append(a.tunnelRoutes, s.Prefix)
			}

		case StepDNS:
			if a.ops.dnsStaticKey != "" {
				if err := a.journal.Record(JournalEntry{
					Kind: a.ops.dnsJournalKind,
					Key:  a.ops.dnsStaticKey,
				}); err != nil {
					return err
				}
			}
			key, err := a.ops.applyDNS(ctx, a.run, s.Iface, s.DNS, a.journal)
			if err != nil {
				return err
			}
			a.dnsKey = key
		}
	}
	return nil
}

func (a *unixApplied) Update(ctx context.Context, d Desired) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	d = NormalizeDesired(d)
	if d.Local != a.desired.Local || d.Peer != a.desired.Peer || d.MTU != a.desired.MTU {
		if err := a.ops.configureInterface(ctx, a.run, a.desired, d); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update interface failed", err)
		}
	}

	newNeedHost := NeedsGatewayHostRoute(d.Routes, d.Gateway)
	var (
		targetSnap       Snapshot
		hasTargetHost    bool
		desiredHostEntry JournalEntry
	)
	if newNeedHost {
		targetSnap = a.snap
		if a.ops.lookupRoute != nil {
			// Skip the tunnel interface and our own prior /32 gateway route so uplink
			// transitions (e.g. Wi-Fi -> hotspot with unchanged gateway IP) refresh nextHop/iface.
			if lookedUp, err := a.ops.lookupRoute(ctx, a.run, d.Gateway, d.Iface, true); err == nil && (lookedUp.GatewayNextHop.IsValid() || lookedUp.GatewayIface != "") {
				targetSnap = lookedUp
			}
		}
		hasTargetHost = targetSnap.GatewayNextHop.IsValid() || targetSnap.GatewayIface != ""
		if hasTargetHost {
			desiredHostEntry = buildHostRouteEntry(d.Gateway, targetSnap.GatewayNextHop, targetSnap.GatewayIface, 0)
		}
	}

	alreadyRecorded := false
	for _, entry := range a.journal.Entries() {
		if entry.Kind != KindHostRoute {
			continue
		}
		if newNeedHost && hasTargetHost && entry == desiredHostEntry {
			alreadyRecorded = true
			continue
		}
		if err := a.ops.deleteHostRoute(ctx, a.run, entry.Gateway, entry.NextHop, entry.Iface); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous host route failed", err)
		}
		if err := a.journal.Remove(entry); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous host route journal failed", err)
		}
	}
	if newNeedHost && hasTargetHost && !alreadyRecorded {
		if err := recordAndApplyHostRoute(a.journal, desiredHostEntry, func() (bool, error) {
			return a.ops.addHostRoute(ctx, a.run, d.Gateway, targetSnap.GatewayNextHop, targetSnap.GatewayIface)
		}); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update gateway host route failed", err)
		}
	}
	if newNeedHost && hasTargetHost {
		a.snap = targetSnap
	}

	toRemove, updatedRoutes, toAdd := DiffRoutes(a.tunnelRoutes, d.Routes)
	for _, r := range toRemove {
		_ = a.ops.deleteTunnelRoute(ctx, a.run, r, a.desired.Iface)
	}
	for _, r := range toAdd {
		created, err := a.ops.addTunnelRoute(ctx, a.run, r, d.Iface)
		if err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: add updated route failed", err)
		}
		if created {
			updatedRoutes = append(updatedRoutes, r)
		}
	}
	a.tunnelRoutes = updatedRoutes

	if HasEffectiveDNS(d.DNS) {
		if a.ops.dnsStaticKey != "" {
			if err := a.journal.Record(a.dnsEntry(d.Iface, a.ops.dnsStaticKey)); err != nil {
				return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: record dns journal failed", err)
			}
		}
		newKey, err := a.ops.applyDNS(ctx, a.run, d.Iface, d.DNS, a.journal)
		if err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
		a.dnsKey = newKey
		if err := a.cleanupDNS(ctx, a.dnsEntry(d.Iface, newKey)); err != nil {
			return err
		}
	} else {
		if err := a.cleanupDNS(ctx, JournalEntry{}); err != nil {
			return err
		}
		a.dnsKey = ""
	}

	a.desired = d
	return nil
}

// EnsureDNS verifies volatile host DNS settings after sleep/wake or link changes
// and restores them if cleared by the operating system.
func (a *unixApplied) EnsureDNS(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ops.ensureDNS == nil || !HasEffectiveDNS(a.desired.DNS) {
		return nil
	}
	key := a.dnsKey
	if key == "" {
		key = a.ops.dnsStaticKey
	}
	newKey, err := a.ops.ensureDNS(ctx, a.run, a.desired.Iface, key, a.desired.DNS, a.journal)
	if err != nil {
		return err
	}
	if newKey != "" {
		a.dnsKey = newKey
		return a.cleanupDNS(ctx, a.dnsEntry(a.desired.Iface, newKey))
	}
	return nil
}

// cleanupDNS retains every unconfirmed mutation, including an earlier failed
// backend switch. A zero keep entry disables all recorded DNS configuration.
func (a *unixApplied) cleanupDNS(ctx context.Context, keep JournalEntry) error {
	for _, entry := range a.journal.Entries() {
		if entry.Kind != a.ops.dnsJournalKind || entry == keep {
			continue
		}
		if err := a.ops.removeDNS(ctx, a.run, entry.Iface, entry.Key); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous DNS configuration failed", err)
		}
		if err := a.journal.Remove(entry); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: remove previous DNS journal failed", err)
		}
	}
	return nil
}

func (a *unixApplied) dnsEntry(iface, key string) JournalEntry {
	if a.ops.dnsStaticKey != "" {
		return JournalEntry{
			Kind: a.ops.dnsJournalKind,
			Key:  key,
		}
	}
	return JournalEntry{
		Kind:  a.ops.dnsJournalKind,
		Iface: iface,
		Key:   key,
	}
}

func (a *unixApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ops.closeOps != nil {
		defer func() { _ = a.ops.closeOps() }()
	}

	for _, route := range slices.Backward(a.tunnelRoutes) {
		_ = a.ops.deleteTunnelRoute(ctx, a.run, route, a.desired.Iface)
	}
	a.tunnelRoutes = nil

	return rollbackUnixJournal(ctx, a.run, a.journal, a.ops)
}

func recoverUnixWithOps(ctx context.Context, journalPath string, run cmdRunner, ops unixPlatformOps) error {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return err
	}
	return rollbackUnixJournal(ctx, run, journal, ops)
}

func rollbackUnixJournal(ctx context.Context, run cmdRunner, journal *Journal, ops unixPlatformOps) error {
	return journal.Rollback(func(entry JournalEntry) error {
		switch entry.Kind {
		case ops.dnsJournalKind:
			return ops.removeDNS(ctx, run, entry.Iface, entry.Key)
		case KindHostRoute:
			return ops.deleteHostRoute(ctx, run, entry.Gateway, entry.NextHop, entry.Iface)
		default:
			return nil
		}
	})
}
