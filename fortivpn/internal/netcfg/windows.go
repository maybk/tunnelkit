//go:build windows

package netcfg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"tunnelkit/fortivpn"
)

// Apply configures the Windows Wintun adapter, gateway host route, tunnel routes,
// and DNS/NRPT settings, recording persistent mutations to journalPath first.
func Apply(ctx context.Context, d Desired, journalPath string) (Applied, error) {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: load journal failed", err)
	}

	luid, err := lookupWindowsLUID(d.Iface)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: resolve adapter LUID failed", err)
	}

	var snap Snapshot
	if NeedsGatewayHostRoute(d.Routes, d.Gateway) {
		snap, err = lookupWindowsGatewayRoute(d.Gateway, luid)
		if err != nil {
			return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: lookup gateway route failed", err)
		}
	}

	ap := &windowsApplied{
		luid:    luid,
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

// Recover rolls back any residual host routes or NRPT registry keys recorded in
// journalPath from a previously crashed helper process.
func Recover(ctx context.Context, journalPath string) error {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return err
	}
	var errs []error
	if err := rollbackWindowsJournal(ctx, journal); err != nil {
		errs = append(errs, err)
	}
	if err := journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

type windowsAdapterOps interface {
	disableIPv6(luid winipcfg.LUID)
	flushTunnelRoutes(luid winipcfg.LUID) error
	configureIPv4Interface(luid winipcfg.LUID, mtu int) error
	setIPv4Address(luid winipcfg.LUID, local netip.Addr) error
	addTunnelRoutes(luid winipcfg.LUID, routes []netip.Prefix) error
	addHostRoute(gwLUID winipcfg.LUID, prefix netip.Prefix, nextHop netip.Addr) (bool, error)
	deleteHostRoute(gwLUID winipcfg.LUID, prefix netip.Prefix, nextHop netip.Addr) error
	applyDNS(ctx context.Context, luid winipcfg.LUID, plan DNSPlan, journal *Journal) error
	removeDNS(ctx context.Context, luid winipcfg.LUID) error
	flushAdapter(luid winipcfg.LUID)
}

type realWindowsAdapterOps struct{}

func (realWindowsAdapterOps) disableIPv6(luid winipcfg.LUID) {
	_ = luid.FlushRoutes(windows.AF_INET6)
	_ = luid.FlushIPAddresses(windows.AF_INET6)
	_ = luid.FlushDNS(windows.AF_INET6)
}

func (realWindowsAdapterOps) flushTunnelRoutes(luid winipcfg.LUID) error {
	return luid.FlushRoutes(windows.AF_INET)
}

func (realWindowsAdapterOps) configureIPv4Interface(luid winipcfg.LUID, mtu int) error {
	ipIf, err := luid.IPInterface(windows.AF_INET)
	if err != nil {
		return fmt.Errorf("get IPInterface: %w", err)
	}
	if mtu > 0 {
		ipIf.NLMTU = uint32(mtu)
	}
	ipIf.UseAutomaticMetric = false
	ipIf.Metric = 1
	if err := ipIf.Set(); err != nil {
		return fmt.Errorf("set IPInterface MTU/Metric: %w", err)
	}
	return nil
}

func (realWindowsAdapterOps) setIPv4Address(luid winipcfg.LUID, local netip.Addr) error {
	prefixes := []netip.Prefix{netip.PrefixFrom(local, 32)}
	err := luid.SetIPAddressesForFamily(windows.AF_INET, prefixes)
	if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
		cleanupAddressesOnDisconnectedInterfaces(windows.AF_INET, prefixes)
		err = luid.SetIPAddressesForFamily(windows.AF_INET, prefixes)
	}
	if err != nil {
		return fmt.Errorf("set interface IP %s: %w", local, err)
	}
	return nil
}

func (realWindowsAdapterOps) addTunnelRoutes(luid winipcfg.LUID, routes []netip.Prefix) error {
	for _, pfx := range routes {
		if !pfx.Addr().Is4() {
			continue
		}
		err := luid.AddRoute(pfx, netip.IPv4Unspecified(), 1)
		if err != nil && !errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return fmt.Errorf("add tunnel route %s: %w", pfx, err)
		}
	}
	return nil
}

func (realWindowsAdapterOps) addHostRoute(gwLUID winipcfg.LUID, prefix netip.Prefix, nextHop netip.Addr) (bool, error) {
	if !nextHop.IsValid() {
		nextHop = netip.IPv4Unspecified()
	}
	if err := gwLUID.AddRoute(prefix, nextHop, 1); err != nil {
		if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return false, nil
		}
		return false, fmt.Errorf("add host route %s: %w", prefix, err)
	}
	return true, nil
}

func (realWindowsAdapterOps) deleteHostRoute(gwLUID winipcfg.LUID, prefix netip.Prefix, nextHop netip.Addr) error {
	if !nextHop.IsValid() {
		nextHop = netip.IPv4Unspecified()
	}
	if err := gwLUID.DeleteRoute(prefix, nextHop); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
		return err
	}
	return nil
}

func (realWindowsAdapterOps) applyDNS(ctx context.Context, luid winipcfg.LUID, plan DNSPlan, journal *Journal) error {
	return applyWindowsDNS(ctx, luid, plan, journal)
}

func (realWindowsAdapterOps) removeDNS(ctx context.Context, luid winipcfg.LUID) error {
	return removeWindowsDNS(ctx, luid)
}

func (realWindowsAdapterOps) flushAdapter(luid winipcfg.LUID) {
	if luid != 0 {
		_ = luid.FlushRoutes(windows.AF_INET)
		_ = luid.FlushDNS(windows.AF_INET)
		_ = luid.FlushIPAddresses(windows.AF_INET)
		_ = luid.FlushRoutes(windows.AF_INET6)
		_ = luid.FlushDNS(windows.AF_INET6)
		_ = luid.FlushIPAddresses(windows.AF_INET6)
	}
}

func cleanupAddressesOnDisconnectedInterfaces(family winipcfg.AddressFamily, addresses []netip.Prefix) {
	if len(addresses) == 0 {
		return
	}
	addrHash := make(map[netip.Addr]bool, len(addresses))
	for i := range addresses {
		addrHash[addresses[i].Addr()] = true
	}
	interfaces, err := winipcfg.GetAdaptersAddresses(family, winipcfg.GAAFlagDefault)
	if err != nil {
		return
	}
	for _, iface := range interfaces {
		if iface.OperStatus == winipcfg.IfOperStatusUp {
			continue
		}
		for address := iface.FirstUnicastAddress; address != nil; address = address.Next {
			if ip, ok := netip.AddrFromSlice(address.Address.IP()); ok && addrHash[ip] {
				prefix := netip.PrefixFrom(ip, int(address.OnLinkPrefixLength))
				_ = iface.LUID.DeleteIPAddress(prefix)
			}
		}
	}
}

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
				entry := JournalEntry{
					Kind:    KindHostRoute,
					Gateway: s.Prefix.Addr().String(),
					LUID:    a.snap.GatewayLUID,
				}
				if s.NextHop.IsValid() {
					entry.NextHop = s.NextHop.String()
				}
				if err := a.journal.Record(entry); err != nil {
					return err
				}
				created, err := ops.addHostRoute(gwLUID, s.Prefix, s.NextHop)
				if err != nil {
					_ = a.journal.Remove(entry)
					return err
				}
				if !created {
					_ = a.journal.Remove(entry)
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

	ops := a.adapterOps()
	oldNeedHost := NeedsGatewayHostRoute(a.desired.Routes, a.desired.Gateway)
	newNeedHost := NeedsGatewayHostRoute(d.Routes, d.Gateway)
	if oldNeedHost && (!newNeedHost || d.Gateway != a.desired.Gateway) {
		for _, entry := range a.journal.Entries() {
			if entry.Kind == KindHostRoute && entry.LUID != 0 && entry.Gateway != "" {
				if gwAddr, err := netip.ParseAddr(entry.Gateway); err == nil {
					nextHop := netip.IPv4Unspecified()
					if entry.NextHop != "" {
						if nh, err := netip.ParseAddr(entry.NextHop); err == nil {
							nextHop = nh
						}
					}
					_ = ops.deleteHostRoute(winipcfg.LUID(entry.LUID), netip.PrefixFrom(gwAddr, 32), nextHop)
				}
				_ = a.journal.Remove(entry)
			}
		}
	}
	if newNeedHost && (!oldNeedHost || d.Gateway != a.desired.Gateway) {
		snap := a.snap
		if snap.GatewayLUID == 0 {
			if lookedUp, err := lookupWindowsGatewayRoute(d.Gateway, a.luid); err == nil {
				snap = lookedUp
				a.snap = snap
			}
		}
		gwLUID := winipcfg.LUID(snap.GatewayLUID)
		if gwLUID != 0 {
			entry := JournalEntry{
				Kind:    KindHostRoute,
				Gateway: d.Gateway.String(),
				LUID:    snap.GatewayLUID,
			}
			if snap.GatewayNextHop.IsValid() {
				entry.NextHop = snap.GatewayNextHop.String()
			}
			if err := a.journal.Record(entry); err != nil {
				return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: record host route journal failed", err)
			}
			created, err := ops.addHostRoute(gwLUID, netip.PrefixFrom(d.Gateway, 32), snap.GatewayNextHop)
			if err != nil {
				_ = a.journal.Remove(entry)
				return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update host route failed", err)
			}
			if !created {
				_ = a.journal.Remove(entry)
			}
		}
	}

	mtu := d.MTU
	if mtu <= 0 {
		mtu = 1400
	}
	if err := a.configureAdapterAndRoutes(d.Local, mtu, d.Routes); err != nil {
		return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update interface and routes failed", err)
	}

	if d.DNS.Mode != fortivpn.DNSOff && len(d.DNS.Servers) > 0 && !SplitDNSNeedsDomains(d.DNS) {
		if err := ops.applyDNS(ctx, a.luid, d.DNS, a.journal); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
	} else {
		_ = ops.removeDNS(ctx, a.luid)
		_ = a.journal.Remove(JournalEntry{
			Kind: KindWindowsNRPT,
			Key:  nrptRulePrefix,
		})
	}

	a.desired = d
	return nil
}

func (a *windowsApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var errs []error
	a.adapterOps().flushAdapter(a.luid)
	if err := rollbackWindowsJournalWithOps(ctx, a.journal, a.adapterOps()); err != nil {
		errs = append(errs, err)
	}
	if err := a.journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func rollbackWindowsJournal(ctx context.Context, journal *Journal) error {
	return rollbackWindowsJournalWithOps(ctx, journal, realWindowsAdapterOps{})
}

func rollbackWindowsJournalWithOps(ctx context.Context, journal *Journal, ops windowsAdapterOps) error {
	var errs []error
	for _, entry := range journal.Entries() {
		switch entry.Kind {
		case KindWindowsNRPT:
			if err := clearWindowsNRPT(); err != nil {
				errs = append(errs, err)
			}
			flushWindowsDNSCache(ctx)
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
					if err := ops.deleteHostRoute(luid, netip.PrefixFrom(gwAddr, 32), nextHop); err != nil {
						errs = append(errs, err)
					}
				}
			}
		}
	}
	return errors.Join(errs...)
}

func lookupWindowsLUID(ifaceName string) (winipcfg.LUID, error) {
	if ifi, err := net.InterfaceByName(ifaceName); err == nil {
		if luid, err := winipcfg.LUIDFromIndex(uint32(ifi.Index)); err == nil {
			return luid, nil
		}
	}
	rows, err := winipcfg.GetIfTable2Ex(winipcfg.MibIfEntryNormal)
	if err != nil {
		return 0, err
	}
	for i := range rows {
		if rows[i].Alias() == ifaceName {
			return rows[i].InterfaceLUID, nil
		}
	}
	return 0, fmt.Errorf("interface %q not found", ifaceName)
}

func lookupWindowsGatewayRoute(gateway netip.Addr, skipLUID winipcfg.LUID) (Snapshot, error) {
	rows, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return Snapshot{}, err
	}

	var (
		found      bool
		bestBits   int = -1
		bestMetric uint64
		bestRow    winipcfg.MibIPforwardRow2
	)
	for _, r := range rows {
		if r.InterfaceLUID == skipLUID || r.Loopback {
			continue
		}
		pfx := r.DestinationPrefix.Prefix()
		if !pfx.IsValid() || !pfx.Contains(gateway) {
			continue
		}
		ipIf, err := r.InterfaceLUID.IPInterface(windows.AF_INET)
		if err != nil || !ipIf.Connected {
			continue
		}
		metric := uint64(r.Metric) + uint64(ipIf.Metric)
		bits := pfx.Bits()
		if !found || bits > bestBits || (bits == bestBits && metric < bestMetric) {
			found = true
			bestBits = bits
			bestMetric = metric
			bestRow = r
		}
	}

	if !found {
		return Snapshot{}, fmt.Errorf("no physical route found for gateway %s", gateway)
	}
	return Snapshot{
		GatewayNextHop: bestRow.NextHop.Addr(),
		GatewayLUID:    uint64(bestRow.InterfaceLUID),
	}, nil
}
