//go:build windows

package netcfg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"tunnelkit/fortivpn"
)

// Apply configures the Windows Wintun adapter, gateway host route, tunnel routes,
// and DNS/NRPT settings, recording persistent mutations to journalPath first.
func Apply(ctx context.Context, d Desired, journalPath string) (Applied, error) {
	luid, err := lookupWindowsLUID(d.Iface)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: resolve adapter LUID failed", err)
	}
	return applyWindowsWithOps(ctx, d, journalPath, uint64(luid), realWindowsAdapterOps{})
}

// Recover rolls back any residual host routes or NRPT registry keys recorded in
// journalPath from a previously crashed helper process.
func Recover(ctx context.Context, journalPath string) error {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return err
	}
	return rollbackWindowsJournalWithOps(ctx, journal, realWindowsAdapterOps{})
}

type realWindowsAdapterOps struct{}

func (realWindowsAdapterOps) lookupGatewayRoute(gateway netip.Addr, skipLUID uint64, skipExistingHostRoute bool) (Snapshot, error) {
	return lookupWindowsGatewayRoute(gateway, winipcfg.LUID(skipLUID), skipExistingHostRoute)
}

func (realWindowsAdapterOps) disableIPv6(luid uint64) {
	w := winipcfg.LUID(luid)
	_ = w.FlushRoutes(windows.AF_INET6)
	_ = w.FlushIPAddresses(windows.AF_INET6)
	_ = w.FlushDNS(windows.AF_INET6)
}

func (realWindowsAdapterOps) flushTunnelRoutes(luid uint64) error {
	return winipcfg.LUID(luid).FlushRoutes(windows.AF_INET)
}

func (realWindowsAdapterOps) configureIPv4Interface(luid uint64, mtu int) error {
	ipIf, err := winipcfg.LUID(luid).IPInterface(windows.AF_INET)
	if err != nil {
		return fmt.Errorf("get IPInterface: %w", err)
	}
	ipIf.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
	ipIf.DadTransmits = 0
	ipIf.ManagedAddressConfigurationSupported = false
	ipIf.OtherStatefulConfigurationSupported = false
	ipIf.DisableDefaultRoutes = false
	ipIf.SitePrefixLength = 0
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

func (realWindowsAdapterOps) disableDNSRegistration(luid uint64) error {
	guid, err := winipcfg.LUID(luid).GUID()
	if err != nil {
		return err
	}
	err = winipcfg.SetInterfaceDnsSettings(*guid, &winipcfg.DnsInterfaceSettings{
		Version: winipcfg.DnsInterfaceSettingsVersion1,
		Flags: winipcfg.DnsInterfaceSettingsFlagRegistrationEnabled |
			winipcfg.DnsInterfaceSettingsFlagRegisterAdapterName,
		RegistrationEnabled: 0,
		RegisterAdapterName: 0,
	})
	if errors.Is(err, windows.ERROR_PROC_NOT_FOUND) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
		return nil
	}
	return err
}

func (realWindowsAdapterOps) setIPv4Address(luid uint64, local netip.Addr) error {
	w := winipcfg.LUID(luid)
	prefixes := []netip.Prefix{netip.PrefixFrom(local, 32)}
	err := w.SetIPAddressesForFamily(windows.AF_INET, prefixes)
	if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
		cleanupAddressesOnDisconnectedInterfaces(windows.AF_INET, prefixes)
		err = w.SetIPAddressesForFamily(windows.AF_INET, prefixes)
	}
	if err != nil {
		return fmt.Errorf("set interface IP %s: %w", local, err)
	}
	return nil
}

func (realWindowsAdapterOps) addTunnelRoutes(luid uint64, routes []netip.Prefix) error {
	w := winipcfg.LUID(luid)
	for _, pfx := range routes {
		if !pfx.Addr().Is4() {
			continue
		}
		err := w.AddRoute(pfx, netip.IPv4Unspecified(), 1)
		if err != nil && !errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return fmt.Errorf("add tunnel route %s: %w", pfx, err)
		}
	}
	return nil
}

func (realWindowsAdapterOps) addHostRoute(gwLUID uint64, prefix netip.Prefix, nextHop netip.Addr) (bool, error) {
	if !nextHop.IsValid() {
		nextHop = netip.IPv4Unspecified()
	}
	if err := winipcfg.LUID(gwLUID).AddRoute(prefix, nextHop, 1); err != nil {
		if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return false, nil
		}
		return false, fmt.Errorf("add host route %s: %w", prefix, err)
	}
	return true, nil
}

func (realWindowsAdapterOps) deleteHostRoute(gwLUID uint64, prefix netip.Prefix, nextHop netip.Addr) error {
	if !nextHop.IsValid() {
		nextHop = netip.IPv4Unspecified()
	}
	if err := winipcfg.LUID(gwLUID).DeleteRoute(prefix, nextHop); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
		return err
	}
	return nil
}

func (realWindowsAdapterOps) applyDNS(ctx context.Context, luid uint64, plan DNSPlan, journal *Journal) error {
	return applyWindowsDNS(ctx, winipcfg.LUID(luid), plan, journal)
}

func (realWindowsAdapterOps) removeDNS(ctx context.Context, luid uint64) error {
	return removeWindowsDNS(ctx, winipcfg.LUID(luid))
}

func (realWindowsAdapterOps) clearNRPT(ctx context.Context) error {
	err := clearWindowsNRPT()
	flushWindowsDNSCache(ctx)
	return err
}

func (realWindowsAdapterOps) flushAdapter(luid uint64) {
	if luid != 0 {
		w := winipcfg.LUID(luid)
		_ = w.FlushRoutes(windows.AF_INET)
		_ = w.FlushDNS(windows.AF_INET)
		_ = w.FlushIPAddresses(windows.AF_INET)
		_ = w.FlushRoutes(windows.AF_INET6)
		_ = w.FlushDNS(windows.AF_INET6)
		_ = w.FlushIPAddresses(windows.AF_INET6)
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

func lookupWindowsLUID(ifaceName string) (winipcfg.LUID, error) {
	target := strings.TrimSpace(ifaceName)
	if ifi, err := net.InterfaceByName(target); err == nil && ifi.Index > 0 {
		if luid, err := winipcfg.LUIDFromIndex(uint32(ifi.Index)); err == nil {
			return luid, nil
		}
	}
	rows, err := winipcfg.GetIfTable2Ex(winipcfg.MibIfEntryNormal)
	if err != nil {
		return 0, err
	}
	for i := range rows {
		if strings.EqualFold(strings.TrimSpace(rows[i].Alias()), target) {
			return rows[i].InterfaceLUID, nil
		}
	}
	return 0, fmt.Errorf("interface %q not found", ifaceName)
}

func lookupWindowsGatewayRoute(gateway netip.Addr, skipLUID winipcfg.LUID, skipExistingHostRoute bool) (Snapshot, error) {
	rows, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return Snapshot{}, err
	}

	gw := gateway.Unmap()
	var (
		found      bool
		bestBits   = -1
		bestMetric uint64
		bestRow    winipcfg.MibIPforwardRow2
	)
	for _, r := range rows {
		if r.InterfaceLUID == skipLUID || r.Loopback {
			continue
		}
		pfx := r.DestinationPrefix.Prefix()
		if !pfx.IsValid() || !pfx.Contains(gw) {
			continue
		}
		if skipExistingHostRoute && pfx.Bits() == 32 && pfx.Addr() == gw {
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
