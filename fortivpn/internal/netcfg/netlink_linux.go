//go:build linux

package netcfg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type realLinuxNetlinkOps struct{}

func defaultLinuxNetlinkOps() linuxNetlinkOps {
	return realLinuxNetlinkOps{}
}

const netlinkDumpMaxAttempts = 3

type linuxLinkMeta struct {
	name     string
	up       bool
	loopback bool
}

// retryNetlinkDump retries bounded netlink dump operations on NLM_F_DUMP_INTR
// (netlink.ErrDumpInterrupted), which the Linux kernel emits when routes or
// interfaces mutate concurrently during a dump (e.g. uplink switch / DHCP).
func retryNetlinkDump[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var (
		res T
		err error
	)
	for range netlinkDumpMaxAttempts {
		if ctx != nil && ctx.Err() != nil {
			var zero T
			return zero, ctx.Err()
		}
		res, err = fn()
		if !errors.Is(err, netlink.ErrDumpInterrupted) {
			return res, err
		}
	}
	return res, err
}

func (realLinuxNetlinkOps) configureInterface(ctx context.Context, iface string, local, peer netip.Addr, mtu int) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("lookup Linux link %q: %w", iface, err)
	}
	addr := buildNetlinkIPv4Addr(local, peer)
	if existing, listErr := retryNetlinkDump(ctx, func() ([]netlink.Addr, error) {
		return netlink.AddrList(link, netlink.FAMILY_V4)
	}); listErr == nil {
		for i := range existing {
			if existing[i].Equal(*addr) {
				continue
			}
			_ = netlink.AddrDel(link, &existing[i])
		}
	}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fmt.Errorf("set Linux addr %s on %s: %w", local, iface, err)
	}
	if mtu > 0 {
		if err := netlink.LinkSetMTU(link, mtu); err != nil {
			return fmt.Errorf("set Linux link %q MTU %d: %w", iface, mtu, err)
		}
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bring up Linux link %q: %w", iface, err)
	}
	return nil
}

func (realLinuxNetlinkOps) lookupRoute(ctx context.Context, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
	if ctx != nil && ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	links, err := retryNetlinkDump(ctx, netlink.LinkList)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list Linux links: %w", err)
	}
	byIndex := make(map[int]linuxLinkMeta, len(links))
	for _, l := range links {
		attrs := l.Attrs()
		if attrs == nil {
			continue
		}
		name := strings.TrimSpace(attrs.Name)
		byIndex[attrs.Index] = linuxLinkMeta{
			name:     name,
			up:       (attrs.Flags & net.FlagUp) != 0,
			loopback: (attrs.Flags&net.FlagLoopback) != 0 || name == "lo",
		}
	}

	filter := &netlink.Route{Table: unix.RT_TABLE_MAIN}
	routes, err := retryNetlinkDump(ctx, func() ([]netlink.Route, error) {
		return netlink.RouteListFiltered(netlink.FAMILY_V4, filter, netlink.RT_FILTER_TABLE)
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("list Linux IPv4 main routes: %w", err)
	}

	snap, ok := selectLinuxGatewaySnapshot(routes, byIndex, gateway, skipIface, skipExistingHostRoute)
	if !ok {
		return Snapshot{}, fmt.Errorf("no physical route found for gateway %s", gateway)
	}
	return snap, nil
}

func selectLinuxGatewaySnapshot(routes []netlink.Route, byIndex map[int]linuxLinkMeta, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, bool) {
	gw := gateway.Unmap()
	if !gw.IsValid() || !gw.Is4() {
		return Snapshot{}, false
	}

	var (
		found        bool
		bestBits     = -1
		bestPriority int
		bestSnap     Snapshot
	)
	for _, r := range routes {
		if r.Type != 0 && r.Type != unix.RTN_UNICAST {
			continue
		}
		pfx, ok := ipNetToIPv4Prefix(r.Dst)
		if !ok || !pfx.Contains(gw) {
			continue
		}
		if skipExistingHostRoute && pfx.Bits() == 32 && pfx.Addr() == gw {
			continue
		}
		linkIdx := r.LinkIndex
		gwIP := r.Gw
		if linkIdx == 0 && len(r.MultiPath) > 0 {
			for _, nh := range r.MultiPath {
				if nh == nil {
					continue
				}
				meta, ok := byIndex[nh.LinkIndex]
				if !ok || !meta.up || meta.loopback || meta.name == "" || (skipIface != "" && meta.name == skipIface) {
					continue
				}
				linkIdx = nh.LinkIndex
				if len(gwIP) == 0 {
					gwIP = nh.Gw
				}
				break
			}
		}
		meta, ok := byIndex[linkIdx]
		if !ok || !meta.up || meta.loopback || meta.name == "" || (skipIface != "" && meta.name == skipIface) {
			continue
		}
		var nextHop netip.Addr
		if parsed, ok := netip.AddrFromSlice(gwIP); ok {
			if u := parsed.Unmap(); u.Is4() && !u.IsUnspecified() {
				nextHop = u
			}
		}
		bits := pfx.Bits()
		if !found || bits > bestBits || (bits == bestBits && r.Priority < bestPriority) {
			found = true
			bestBits = bits
			bestPriority = r.Priority
			bestSnap = Snapshot{
				GatewayNextHop: nextHop,
				GatewayIface:   meta.name,
			}
		}
	}
	return bestSnap, found
}

func (realLinuxNetlinkOps) addHostRoute(ctx context.Context, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
	if ctx != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}
	if !nextHop.IsValid() && iface == "" {
		return false, fmt.Errorf("missing nextHop or interface for host route %s", host)
	}
	rt := &netlink.Route{
		Dst: prefixToIPv4Net(netip.PrefixFrom(host.Unmap(), 32)),
	}
	if nextHop.IsValid() && nextHop.Is4() {
		rt.Gw = net.IP(nextHop.Unmap().AsSlice())
	}
	if iface != "" {
		link, err := netlink.LinkByName(iface)
		if err != nil {
			return false, fmt.Errorf("lookup Linux link %q: %w", iface, err)
		}
		rt.LinkIndex = link.Attrs().Index
	}
	if err := netlink.RouteAdd(rt); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return false, nil
		}
		return false, fmt.Errorf("add Linux host route %s: %w", host, err)
	}
	return true, nil
}

func (realLinuxNetlinkOps) deleteHostRoute(ctx context.Context, host string, nextHop string, iface string) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if host == "" {
		return nil
	}
	gwAddr, err := netip.ParseAddr(strings.TrimSuffix(host, "/32"))
	if err != nil || !gwAddr.Is4() {
		return nil
	}
	rt := &netlink.Route{
		Dst: prefixToIPv4Net(netip.PrefixFrom(gwAddr.Unmap(), 32)),
	}
	if nextHop != "" {
		if nh, parseErr := netip.ParseAddr(nextHop); parseErr == nil && nh.Is4() {
			rt.Gw = net.IP(nh.Unmap().AsSlice())
		}
	}
	if iface != "" {
		link, err := netlink.LinkByName(iface)
		if err != nil {
			if isLinuxNotFoundErr(err) {
				return nil
			}
			return fmt.Errorf("lookup Linux link %q: %w", iface, err)
		}
		rt.LinkIndex = link.Attrs().Index
	}
	if err := netlink.RouteDel(rt); err != nil && !isLinuxNotFoundErr(err) {
		return fmt.Errorf("delete Linux host route %s: %w", host, err)
	}
	return nil
}

func (realLinuxNetlinkOps) addTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) (bool, error) {
	if ctx != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return false, fmt.Errorf("lookup Linux link %q: %w", iface, err)
	}
	rt := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       prefixToIPv4Net(prefix.Masked()),
	}
	if err := netlink.RouteAdd(rt); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return false, nil
		}
		return false, fmt.Errorf("add Linux tunnel route %s on %s: %w", prefix, iface, err)
	}
	return true, nil
}

func (realLinuxNetlinkOps) deleteTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	link, err := netlink.LinkByName(iface)
	if err != nil {
		if isLinuxNotFoundErr(err) {
			return nil
		}
		return fmt.Errorf("lookup Linux link %q: %w", iface, err)
	}
	rt := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       prefixToIPv4Net(prefix.Masked()),
	}
	if err := netlink.RouteDel(rt); err != nil && !isLinuxNotFoundErr(err) {
		return fmt.Errorf("delete Linux tunnel route %s on %s: %w", prefix, iface, err)
	}
	return nil
}

func (realLinuxNetlinkOps) close() error { return nil }

func buildNetlinkIPv4Addr(local, peer netip.Addr) *netlink.Addr {
	addr := &netlink.Addr{
		IPNet: prefixToIPv4Net(netip.PrefixFrom(local.Unmap(), 32)),
	}
	if peer.IsValid() && !peer.IsUnspecified() && peer.Is4() {
		addr.Peer = prefixToIPv4Net(netip.PrefixFrom(peer.Unmap(), 32))
	}
	return addr
}

func prefixToIPv4Net(p netip.Prefix) *net.IPNet {
	u := p.Addr().Unmap()
	return &net.IPNet{
		IP:   net.IP(u.AsSlice()),
		Mask: net.CIDRMask(p.Bits(), 32),
	}
}

func ipNetToIPv4Prefix(ipNet *net.IPNet) (netip.Prefix, bool) {
	if ipNet == nil || len(ipNet.IP) == 0 {
		return netip.PrefixFrom(netip.IPv4Unspecified(), 0), true
	}
	addr, ok := netip.AddrFromSlice(ipNet.IP)
	if !ok {
		return netip.Prefix{}, false
	}
	u := addr.Unmap()
	if !u.Is4() {
		return netip.Prefix{}, false
	}
	ones, total := ipNet.Mask.Size()
	if total != 32 {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(u, ones).Masked(), true
}

func isLinuxNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	var linkNotFound netlink.LinkNotFoundError
	return errors.As(err, &linkNotFound) ||
		errors.Is(err, unix.ENOENT) ||
		errors.Is(err, unix.ESRCH) ||
		errors.Is(err, unix.ENODEV) ||
		errors.Is(err, unix.EADDRNOTAVAIL) ||
		errors.Is(err, os.ErrNotExist)
}
