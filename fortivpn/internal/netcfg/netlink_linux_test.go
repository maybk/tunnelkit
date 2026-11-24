//go:build linux

package netcfg

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func TestLinuxNetlinkHelpers(t *testing.T) {
	addr := buildNetlinkIPv4Addr(netip.MustParseAddr("10.200.0.2"), netip.MustParseAddr("169.254.2.1"))
	if addr.IPNet == nil || addr.IPNet.String() != "10.200.0.2/32" {
		t.Fatalf("unexpected local addr: %v", addr.IPNet)
	}
	if addr.Peer == nil || addr.Peer.String() != "169.254.2.1/32" {
		t.Fatalf("unexpected peer addr: %v", addr.Peer)
	}

	pfx, ok := ipNetToIPv4Prefix(nil)
	if !ok || pfx != netip.MustParsePrefix("0.0.0.0/0") {
		t.Fatalf("nil IPNet = (%v, %v), want (0.0.0.0/0, true)", pfx, ok)
	}
	pfx, ok = ipNetToIPv4Prefix(&net.IPNet{IP: net.IPv4(10, 10, 0, 5), Mask: net.CIDRMask(16, 32)})
	if !ok || pfx != netip.MustParsePrefix("10.10.0.0/16") {
		t.Fatalf("masked IPv4 prefix = (%v, %v), want (10.10.0.0/16, true)", pfx, ok)
	}
	if _, ok := ipNetToIPv4Prefix(&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)}); ok {
		t.Fatal("IPv6 prefix must be ignored by ipNetToIPv4Prefix")
	}

	if !isLinuxNotFoundErr(netlink.LinkNotFoundError{}) || !isLinuxNotFoundErr(unix.ESRCH) {
		t.Fatal("expected LinkNotFoundError and ESRCH to match isLinuxNotFoundErr")
	}
	if isLinuxNotFoundErr(unix.EACCES) {
		t.Fatal("EACCES must not be treated as not-found")
	}
}

func TestSelectLinuxGatewaySnapshotLPMMetricAndSkipExistingHostRoute(t *testing.T) {
	byIndex := map[int]linuxLinkMeta{
		1: {name: "lo", up: true, loopback: true},
		2: {name: "eth0", up: true},
		3: {name: "wlan0", up: true},
		7: {name: "forti0", up: true},
	}
	gw := netip.MustParseAddr("198.51.100.10")
	routes := []netlink.Route{
		// Tunnel /1 route on forti0 must be ignored.
		{Type: unix.RTN_UNICAST, LinkIndex: 7, Dst: prefixToIPv4Net(netip.MustParsePrefix("128.0.0.0/1")), Priority: 1},
		// Stale /32 host route on eth0.
		{Type: unix.RTN_UNICAST, LinkIndex: 2, Dst: prefixToIPv4Net(netip.MustParsePrefix("198.51.100.10/32")), Gw: net.IPv4(192, 168, 1, 1), Priority: 50},
		// Higher metric default on eth0.
		{Type: unix.RTN_UNICAST, LinkIndex: 2, Dst: nil, Gw: net.IPv4(192, 168, 1, 1), Priority: 600},
		// Lower metric multipath default on wlan0 (new active uplink).
		{Type: unix.RTN_UNICAST, Dst: nil, Priority: 100, MultiPath: []*netlink.NexthopInfo{{LinkIndex: 3, Gw: net.IPv4(172, 20, 10, 1)}}},
	}

	initial, ok := selectLinuxGatewaySnapshot(routes, byIndex, gw, "forti0", false)
	if !ok || initial.GatewayIface != "eth0" || initial.GatewayNextHop != netip.MustParseAddr("192.168.1.1") {
		t.Fatalf("unexpected initial lookup: ok=%v snap=%+v", ok, initial)
	}

	refreshed, ok := selectLinuxGatewaySnapshot(routes, byIndex, gw, "forti0", true)
	if !ok || refreshed.GatewayIface != "wlan0" || refreshed.GatewayNextHop != netip.MustParseAddr("172.20.10.1") {
		t.Fatalf("unexpected refreshed lookup: ok=%v snap=%+v", ok, refreshed)
	}
}

func TestRetryNetlinkDump(t *testing.T) {
	attempts := 0
	got, err := retryNetlinkDump(context.Background(), func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, netlink.ErrDumpInterrupted
		}
		return 42, nil
	})
	if err != nil || got != 42 || attempts != 3 {
		t.Fatalf("retry success = (%d, %v, attempts=%d), want (42, nil, 3)", got, err, attempts)
	}

	attempts = 0
	_, err = retryNetlinkDump(context.Background(), func() (int, error) {
		attempts++
		return 0, netlink.ErrDumpInterrupted
	})
	if !errors.Is(err, netlink.ErrDumpInterrupted) || attempts != netlinkDumpMaxAttempts {
		t.Fatalf("retry exhaustion = (%v, attempts=%d), want (ErrDumpInterrupted, %d)", err, attempts, netlinkDumpMaxAttempts)
	}

	attempts = 0
	_, err = retryNetlinkDump(context.Background(), func() (int, error) {
		attempts++
		return 0, unix.EPERM
	})
	if !errors.Is(err, unix.EPERM) || attempts != 1 {
		t.Fatalf("non-interrupt error must not retry: err=%v attempts=%d", err, attempts)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts = 0
	_, err = retryNetlinkDump(canceledCtx, func() (int, error) {
		attempts++
		return 1, nil
	})
	if !errors.Is(err, context.Canceled) || attempts != 0 {
		t.Fatalf("canceled context must abort immediately: err=%v attempts=%d", err, attempts)
	}
}
