//go:build darwin

package netcfg

import (
	"net/netip"
	"testing"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func TestBuildDarwinRouteMessage(t *testing.T) {
	msg, err := buildDarwinRouteMessage(
		unix.RTM_ADD,
		netip.MustParsePrefix("198.51.100.10/32"),
		netip.MustParseAddr("192.168.1.1"),
		"en0",
		0,
		1234,
		7,
	)
	if err != nil {
		t.Fatalf("buildDarwinRouteMessage host route failed: %v", err)
	}
	if msg.Flags&(unix.RTF_UP|unix.RTF_STATIC|unix.RTF_HOST|unix.RTF_GATEWAY) != (unix.RTF_UP | unix.RTF_STATIC | unix.RTF_HOST | unix.RTF_GATEWAY) {
		t.Fatalf("unexpected host route flags: %#x", msg.Flags)
	}
	if msg.Addrs[unix.RTAX_NETMASK] != nil {
		t.Fatalf("expected nil netmask for /32 host route")
	}
	if _, err := msg.Marshal(); err != nil {
		t.Fatalf("Marshal host route failed: %v", err)
	}

	tunMsg, err := buildDarwinRouteMessage(
		unix.RTM_ADD,
		netip.MustParsePrefix("10.20.0.0/16"),
		netip.Addr{},
		"utun7",
		12,
		1234,
		8,
	)
	if err != nil {
		t.Fatalf("buildDarwinRouteMessage tunnel route failed: %v", err)
	}
	if tunMsg.Index != 12 || tunMsg.Flags&unix.RTF_HOST != 0 || tunMsg.Flags&unix.RTF_GATEWAY != 0 {
		t.Fatalf("unexpected tunnel route message: index=%d flags=%#x", tunMsg.Index, tunMsg.Flags)
	}
	mask, ok := tunMsg.Addrs[unix.RTAX_NETMASK].(*route.Inet4Addr)
	if !ok || mask.IP != [4]byte{255, 255, 0, 0} {
		t.Fatalf("unexpected tunnel netmask: %#v", tunMsg.Addrs[unix.RTAX_NETMASK])
	}
	if _, err := tunMsg.Marshal(); err != nil {
		t.Fatalf("Marshal tunnel route failed: %v", err)
	}
}

func TestSelectDarwinGatewaySnapshotLPMAndSkipExistingHostRoute(t *testing.T) {
	ifaces := map[int]darwinIfMeta{
		1: {name: "lo0", up: true, loopback: true},
		4: {name: "en0", up: true},
		5: {name: "en1", up: true},
		9: {name: "utun7", up: true},
	}

	makeMsg := func(flags, idx int, dst, mask, gw [4]byte, hasMask, hasGW bool) *route.RouteMessage {
		addrs := make([]route.Addr, unix.RTAX_NETMASK+1)
		addrs[unix.RTAX_DST] = &route.Inet4Addr{IP: dst}
		if hasMask {
			addrs[unix.RTAX_NETMASK] = &route.Inet4Addr{IP: mask}
		}
		if hasGW {
			addrs[unix.RTAX_GATEWAY] = &route.Inet4Addr{IP: gw}
		}
		return &route.RouteMessage{Flags: flags, Index: idx, Addrs: addrs}
	}

	gw := netip.MustParseAddr("198.51.100.10")
	msgs := []route.Message{
		// Tunnel route covering 128.0.0.0/1 on utun7 must be skipped.
		makeMsg(unix.RTF_UP, 9, [4]byte{128, 0, 0, 0}, [4]byte{128, 0, 0, 0}, [4]byte{}, true, false),
		// Stale /32 host route on en0.
		makeMsg(unix.RTF_UP|unix.RTF_HOST|unix.RTF_GATEWAY, 4, [4]byte{198, 51, 100, 10}, [4]byte{}, [4]byte{192, 168, 1, 1}, false, true),
		// Scoped default on en0 should lose to global default on en1.
		makeMsg(unix.RTF_UP|unix.RTF_GATEWAY|darwinRTFIfScope, 4, [4]byte{0, 0, 0, 0}, [4]byte{0, 0, 0, 0}, [4]byte{192, 168, 1, 1}, true, true),
		// Global default on en1 (new active hotspot uplink).
		makeMsg(unix.RTF_UP|unix.RTF_GATEWAY, 5, [4]byte{0, 0, 0, 0}, [4]byte{0, 0, 0, 0}, [4]byte{172, 20, 10, 1}, true, true),
	}

	// Without skipping /32 host route, LPM returns the /32 route on en0.
	snap, ok := selectDarwinGatewaySnapshot(msgs, ifaces, gw, "utun7", false)
	if !ok || snap.GatewayIface != "en0" || snap.GatewayNextHop != netip.MustParseAddr("192.168.1.1") {
		t.Fatalf("unexpected initial lookup: ok=%v snap=%+v", ok, snap)
	}

	// When refreshing on reconnect (skipExistingHostRoute=true), it ignores stale /32 and picks global default on en1.
	refreshed, ok := selectDarwinGatewaySnapshot(msgs, ifaces, gw, "utun7", true)
	if !ok || refreshed.GatewayIface != "en1" || refreshed.GatewayNextHop != netip.MustParseAddr("172.20.10.1") {
		t.Fatalf("unexpected refreshed lookup: ok=%v snap=%+v", ok, refreshed)
	}
}
