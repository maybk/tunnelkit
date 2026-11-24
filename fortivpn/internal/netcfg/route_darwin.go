//go:build darwin

package netcfg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

// darwinRTFIfScope matches XNU <net/route.h> RTF_IFSCOPE (interface-scoped route).
const darwinRTFIfScope = 0x1000000

type darwinIfMeta struct {
	name     string
	up       bool
	loopback bool
}

// darwinNativeRouteOps uses macOS PF_ROUTE sockets for route mutations and
// sysctl(NET_RT_DUMP) via x/net/route for IPv4 gateway route lookup, avoiding
// repeated /sbin/route subprocess spawns and text output parsing.
type darwinNativeRouteOps struct {
	mu      sync.Mutex
	fd      int
	ifIndex map[string]int
	seq     atomic.Int32
}

func defaultDarwinRouteOps() darwinRouteOps {
	return &darwinNativeRouteOps{
		fd:      -1,
		ifIndex: make(map[string]int),
	}
}

func (s *darwinNativeRouteOps) ensureFDLocked() (int, error) {
	if s.fd >= 0 {
		return s.fd, nil
	}
	syscall.ForkLock.RLock()
	fd, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
	if err == nil {
		unix.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return -1, fmt.Errorf("open Darwin PF_ROUTE socket: %w", err)
	}
	// Disable loopback echoes of our own route messages to avoid ENOBUFS during batch updates.
	_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_USELOOPBACK, 0)
	s.fd = fd
	return fd, nil
}

func (s *darwinNativeRouteOps) lookupIfIndexLocked(name string) (int, error) {
	if idx, ok := s.ifIndex[name]; ok && idx > 0 {
		return idx, nil
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return 0, err
	}
	s.ifIndex[name] = ifi.Index
	return ifi.Index, nil
}

func (s *darwinNativeRouteOps) lookupRoute(ctx context.Context, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
	if ctx != nil && ctx.Err() != nil {
		return Snapshot{}, ctx.Err()
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return Snapshot{}, fmt.Errorf("list Darwin interfaces: %w", err)
	}
	ifByIndex := make(map[int]darwinIfMeta, len(ifaces))
	for _, ifi := range ifaces {
		ifByIndex[ifi.Index] = darwinIfMeta{
			name:     strings.TrimSpace(ifi.Name),
			up:       (ifi.Flags & net.FlagUp) != 0,
			loopback: (ifi.Flags & net.FlagLoopback) != 0,
		}
	}

	rib, err := route.FetchRIB(unix.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch Darwin IPv4 RIB: %w", err)
	}
	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse Darwin IPv4 RIB: %w", err)
	}
	snap, ok := selectDarwinGatewaySnapshot(msgs, ifByIndex, gateway, skipIface, skipExistingHostRoute)
	if !ok {
		return Snapshot{}, fmt.Errorf("no physical route found for gateway %s", gateway)
	}
	return snap, nil
}

func (s *darwinNativeRouteOps) addHostRoute(ctx context.Context, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
	if !host.IsValid() || !host.Is4() {
		return false, fmt.Errorf("invalid Darwin host route target %v", host)
	}
	if !nextHop.IsValid() && iface == "" {
		return false, fmt.Errorf("missing nextHop or interface for host route %s", host)
	}
	return s.writeRoute(ctx, unix.RTM_ADD, netip.PrefixFrom(host.Unmap(), 32), nextHop, iface)
}

func (s *darwinNativeRouteOps) deleteHostRoute(ctx context.Context, host string, nextHop string, iface string) error {
	if host == "" {
		return nil
	}
	gwAddr, err := netip.ParseAddr(strings.TrimSuffix(host, "/32"))
	if err != nil || !gwAddr.Is4() {
		return nil
	}
	var nhAddr netip.Addr
	if nextHop != "" {
		if parsed, parseErr := netip.ParseAddr(nextHop); parseErr == nil && parsed.Is4() {
			nhAddr = parsed
		}
	}
	_, err = s.writeRoute(ctx, unix.RTM_DELETE, netip.PrefixFrom(gwAddr.Unmap(), 32), nhAddr, iface)
	return err
}

func (s *darwinNativeRouteOps) addTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) (bool, error) {
	if !prefix.IsValid() || !prefix.Addr().Is4() || iface == "" {
		return false, fmt.Errorf("invalid Darwin tunnel route %v on %q", prefix, iface)
	}
	return s.writeRoute(ctx, unix.RTM_ADD, prefix.Masked(), netip.Addr{}, iface)
}

func (s *darwinNativeRouteOps) deleteTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) error {
	if !prefix.IsValid() || !prefix.Addr().Is4() {
		return nil
	}
	_, err := s.writeRoute(ctx, unix.RTM_DELETE, prefix.Masked(), netip.Addr{}, iface)
	return err
}

func (s *darwinNativeRouteOps) writeRoute(ctx context.Context, msgType int, prefix netip.Prefix, nextHop netip.Addr, iface string) (bool, error) {
	if ctx != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var ifIdx int
	if !nextHop.IsValid() && iface != "" {
		idx, err := s.lookupIfIndexLocked(iface)
		if err != nil {
			if msgType == unix.RTM_DELETE {
				// During crash recovery a temporary utun interface may already be gone.
				ifIdx = 0
			} else {
				return false, fmt.Errorf("resolve Darwin interface %q index for route %s: %w", iface, prefix, err)
			}
		} else {
			ifIdx = idx
		}
	}

	msg, err := buildDarwinRouteMessage(msgType, prefix, nextHop, iface, ifIdx, os.Getpid(), int(s.seq.Add(1)))
	if err != nil {
		return false, err
	}
	raw, err := msg.Marshal()
	if err != nil {
		return false, fmt.Errorf("marshal Darwin route message (%s): %w", prefix, err)
	}
	fd, err := s.ensureFDLocked()
	if err != nil {
		return false, err
	}
	if _, err := writeDarwinRouteMessage(fd, raw); err != nil {
		if msgType == unix.RTM_ADD && errors.Is(err, unix.EEXIST) {
			return false, nil
		}
		if msgType == unix.RTM_DELETE && isDarwinRouteNotFoundErr(err) {
			return false, nil
		}
		return false, fmt.Errorf("Darwin PF_ROUTE op=%d prefix=%s: %w", msgType, prefix, err)
	}
	return true, nil
}

func (s *darwinNativeRouteOps) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.ifIndex)
	if s.fd < 0 {
		return nil
	}
	fd := s.fd
	s.fd = -1
	return unix.Close(fd)
}

func writeDarwinRouteMessage(fd int, raw []byte) (int, error) {
	for {
		n, err := unix.Write(fd, raw)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return n, err
	}
}

func isDarwinRouteNotFoundErr(err error) bool {
	return errors.Is(err, unix.ESRCH) || errors.Is(err, unix.ENXIO) || errors.Is(err, unix.ENODEV)
}

func buildDarwinRouteMessage(msgType int, prefix netip.Prefix, nextHop netip.Addr, iface string, ifIndex int, pid int, seq int) (*route.RouteMessage, error) {
	if !prefix.IsValid() || !prefix.Addr().Is4() {
		return nil, fmt.Errorf("invalid Darwin IPv4 route prefix %v", prefix)
	}
	pfx := prefix.Masked()
	addr := pfx.Addr().Unmap()
	isHost := pfx.Bits() == 32

	flags := unix.RTF_UP | unix.RTF_STATIC
	if isHost {
		flags |= unix.RTF_HOST
	}

	addrs := make([]route.Addr, unix.RTAX_NETMASK+1)
	addrs[unix.RTAX_DST] = &route.Inet4Addr{IP: addr.As4()}
	if !isHost {
		addrs[unix.RTAX_NETMASK] = &route.Inet4Addr{IP: cidrMask4(pfx.Bits())}
	}

	var msgIndex int
	if nextHop.IsValid() && nextHop.Is4() {
		flags |= unix.RTF_GATEWAY
		addrs[unix.RTAX_GATEWAY] = &route.Inet4Addr{IP: nextHop.Unmap().As4()}
	} else if msgType == unix.RTM_ADD || ifIndex > 0 {
		msgIndex = ifIndex
		addrs[unix.RTAX_GATEWAY] = &route.LinkAddr{Index: ifIndex, Name: iface}
	}

	return &route.RouteMessage{
		Version: unix.RTM_VERSION,
		Type:    msgType,
		Flags:   flags,
		Index:   msgIndex,
		ID:      uintptr(pid),
		Seq:     seq,
		Addrs:   addrs,
	}, nil
}

func selectDarwinGatewaySnapshot(msgs []route.Message, ifByIndex map[int]darwinIfMeta, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, bool) {
	gw := gateway.Unmap()
	if !gw.IsValid() || !gw.Is4() {
		return Snapshot{}, false
	}

	var (
		found      bool
		bestBits   = -1
		bestScoped = true
		bestSnap   Snapshot
	)

	for _, msg := range msgs {
		rm, ok := msg.(*route.RouteMessage)
		if !ok {
			continue
		}
		if rm.Flags&unix.RTF_UP == 0 {
			continue
		}
		if rm.Flags&(unix.RTF_LLINFO|unix.RTF_WASCLONED|unix.RTF_REJECT|unix.RTF_BLACKHOLE) != 0 {
			continue
		}
		iface, ok := resolveDarwinRouteIface(rm, ifByIndex)
		if !ok || (skipIface != "" && iface == skipIface) {
			continue
		}
		pfx, ok := parseDarwinIPv4Prefix(rm)
		if !ok || !pfx.Contains(gw) {
			continue
		}
		if skipExistingHostRoute && pfx.Bits() == 32 && pfx.Addr() == gw {
			continue
		}
		nextHop := parseDarwinIPv4Gateway(rm)
		scoped := rm.Flags&darwinRTFIfScope != 0
		bits := pfx.Bits()
		// Prefer global (non-scoped) routes over interface-scoped routes, then longest prefix match.
		if !found || (bestScoped && !scoped) || (scoped == bestScoped && bits > bestBits) {
			found = true
			bestBits = bits
			bestScoped = scoped
			bestSnap = Snapshot{
				GatewayNextHop: nextHop,
				GatewayIface:   iface,
			}
		}
	}
	return bestSnap, found
}

func resolveDarwinRouteIface(rm *route.RouteMessage, ifByIndex map[int]darwinIfMeta) (string, bool) {
	if rm.Index > 0 {
		if meta, ok := ifByIndex[rm.Index]; ok {
			if !meta.up || meta.loopback || meta.name == "" {
				return "", false
			}
			return meta.name, true
		}
	}
	if len(rm.Addrs) > unix.RTAX_GATEWAY {
		if la, ok := rm.Addrs[unix.RTAX_GATEWAY].(*route.LinkAddr); ok {
			if la.Index > 0 {
				if meta, found := ifByIndex[la.Index]; found {
					if !meta.up || meta.loopback || meta.name == "" {
						return "", false
					}
					return meta.name, true
				}
			}
			name := strings.TrimSpace(la.Name)
			if name != "" && !strings.HasPrefix(name, "lo") {
				return name, true
			}
		}
	}
	return "", false
}

func parseDarwinIPv4Prefix(rm *route.RouteMessage) (netip.Prefix, bool) {
	if len(rm.Addrs) <= unix.RTAX_DST || rm.Addrs[unix.RTAX_DST] == nil {
		return netip.Prefix{}, false
	}
	dst, ok := rm.Addrs[unix.RTAX_DST].(*route.Inet4Addr)
	if !ok {
		return netip.Prefix{}, false
	}
	addr := netip.AddrFrom4(dst.IP)
	if rm.Flags&unix.RTF_HOST != 0 {
		return netip.PrefixFrom(addr, 32), true
	}
	var maskAddr route.Addr
	if len(rm.Addrs) > unix.RTAX_NETMASK {
		maskAddr = rm.Addrs[unix.RTAX_NETMASK]
	}
	if maskAddr == nil {
		if addr.IsUnspecified() {
			return netip.PrefixFrom(netip.IPv4Unspecified(), 0), true
		}
		return netip.Prefix{}, false
	}
	m4, ok := maskAddr.(*route.Inet4Addr)
	if !ok {
		return netip.Prefix{}, false
	}
	ones, total := net.IPMask(m4.IP[:]).Size()
	if total != 32 {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, ones).Masked(), true
}

func parseDarwinIPv4Gateway(rm *route.RouteMessage) netip.Addr {
	if len(rm.Addrs) <= unix.RTAX_GATEWAY || rm.Addrs[unix.RTAX_GATEWAY] == nil {
		return netip.Addr{}
	}
	gw, ok := rm.Addrs[unix.RTAX_GATEWAY].(*route.Inet4Addr)
	if !ok {
		return netip.Addr{}
	}
	addr := netip.AddrFrom4(gw.IP)
	if addr.IsUnspecified() {
		return netip.Addr{}
	}
	return addr
}

func cidrMask4(bits int) [4]byte {
	var b [4]byte
	for i := range 4 {
		n := min(max(bits-i*8, 0), 8)
		b[i] = byte(0xff << (8 - n))
	}
	return b
}
