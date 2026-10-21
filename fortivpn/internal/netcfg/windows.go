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

type windowsApplied struct {
	mu      sync.Mutex
	luid    winipcfg.LUID
	journal *Journal
	desired Desired
	snap    Snapshot
}

func (a *windowsApplied) executeSteps(ctx context.Context, steps []Step) error {
	var routeData []*winipcfg.RouteData
	for _, s := range steps {
		switch s.Kind {
		case StepConfigureInterface:
			if err := a.configureInterface(s.Local, s.MTU); err != nil {
				return err
			}

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
				nextHop := s.NextHop
				if !nextHop.IsValid() {
					nextHop = netip.IPv4Unspecified()
				}
				if err := gwLUID.AddRoute(s.Prefix, nextHop, 1); err != nil {
					_ = a.journal.Remove(entry)
					if !errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
						return fmt.Errorf("add host route %s: %w", s.Prefix, err)
					}
				}
			}

		case StepTunnelRoute:
			routeData = append(routeData, &winipcfg.RouteData{
				Destination: s.Prefix,
				NextHop:     netip.IPv4Unspecified(),
				Metric:      1,
			})

		case StepDNS:
			if len(routeData) > 0 {
				if err := a.luid.SetRoutesForFamily(windows.AF_INET, routeData); err != nil {
					return fmt.Errorf("set tunnel routes: %w", err)
				}
				routeData = nil
			}
			if err := applyWindowsDNS(ctx, a.luid, s.DNS, a.journal); err != nil {
				return err
			}
		}
	}

	if len(routeData) > 0 {
		if err := a.luid.SetRoutesForFamily(windows.AF_INET, routeData); err != nil {
			return fmt.Errorf("set tunnel routes: %w", err)
		}
	}
	return nil
}

func (a *windowsApplied) configureInterface(local netip.Addr, mtu int) error {
	if err := a.luid.SetIPAddressesForFamily(windows.AF_INET, []netip.Prefix{netip.PrefixFrom(local, 32)}); err != nil {
		return fmt.Errorf("set interface IP %s: %w", local, err)
	}
	ipIf, err := a.luid.IPInterface(windows.AF_INET)
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

func (a *windowsApplied) Update(ctx context.Context, d Desired) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	mtu := d.MTU
	if mtu <= 0 {
		mtu = 1400
	}
	if d.Local != a.desired.Local || mtu != a.desired.MTU {
		if err := a.configureInterface(d.Local, mtu); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update interface failed", err)
		}
	}

	routeData := make([]*winipcfg.RouteData, 0, len(d.Routes))
	for _, r := range d.Routes {
		routeData = append(routeData, &winipcfg.RouteData{
			Destination: r,
			NextHop:     netip.IPv4Unspecified(),
			Metric:      1,
		})
	}
	if err := a.luid.SetRoutesForFamily(windows.AF_INET, routeData); err != nil {
		return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update routes failed", err)
	}

	if d.DNS.Mode != fortivpn.DNSOff && len(d.DNS.Servers) > 0 && !SplitDNSNeedsDomains(d.DNS) {
		if err := applyWindowsDNS(ctx, a.luid, d.DNS, a.journal); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
	} else {
		_ = removeWindowsDNS(ctx, a.luid)
	}

	a.desired = d
	return nil
}

func (a *windowsApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var errs []error
	if a.luid != 0 {
		_ = a.luid.FlushRoutes(windows.AF_INET)
		_ = a.luid.FlushDNS(windows.AF_INET)
		_ = a.luid.FlushIPAddresses(windows.AF_INET)
	}
	if err := rollbackWindowsJournal(ctx, a.journal); err != nil {
		errs = append(errs, err)
	}
	if err := a.journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func rollbackWindowsJournal(ctx context.Context, journal *Journal) error {
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
					if err := luid.DeleteRoute(netip.PrefixFrom(gwAddr, 32), nextHop); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
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
		bestMetric uint32
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
		metric := r.Metric
		if ipIf, err := r.InterfaceLUID.IPInterface(windows.AF_INET); err == nil {
			if !ipIf.Connected {
				continue
			}
			metric += ipIf.Metric
		}
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
