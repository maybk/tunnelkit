//go:build windows

package netcfg

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"tunnelkit/fortivpn"
)

type fakeWindowsOps struct {
	calls           []string
	hostRouteExists bool
	appliedRoutes   []netip.Prefix
}

func (f *fakeWindowsOps) disableIPv6(_ winipcfg.LUID) {
	f.calls = append(f.calls, "disableIPv6")
}

func (f *fakeWindowsOps) flushTunnelRoutes(_ winipcfg.LUID) error {
	f.calls = append(f.calls, "flushTunnelRoutes")
	f.appliedRoutes = nil
	return nil
}

func (f *fakeWindowsOps) configureIPv4Interface(_ winipcfg.LUID, _ int) error {
	f.calls = append(f.calls, "configureIPv4Interface")
	return nil
}

func (f *fakeWindowsOps) setIPv4Address(_ winipcfg.LUID, local netip.Addr) error {
	f.calls = append(f.calls, "setIPv4Address:"+local.String())
	return nil
}

func (f *fakeWindowsOps) addTunnelRoutes(_ winipcfg.LUID, routes []netip.Prefix) error {
	f.calls = append(f.calls, "addTunnelRoutes")
	f.appliedRoutes = append([]netip.Prefix(nil), routes...)
	return nil
}

func (f *fakeWindowsOps) addHostRoute(_ winipcfg.LUID, prefix netip.Prefix, _ netip.Addr) (bool, error) {
	f.calls = append(f.calls, "addHostRoute:"+prefix.String())
	if f.hostRouteExists {
		return false, nil
	}
	return true, nil
}

func (f *fakeWindowsOps) deleteHostRoute(_ winipcfg.LUID, prefix netip.Prefix, _ netip.Addr) error {
	f.calls = append(f.calls, "deleteHostRoute:"+prefix.String())
	return nil
}

func (f *fakeWindowsOps) applyDNS(_ context.Context, _ winipcfg.LUID, _ DNSPlan, journal *Journal) error {
	f.calls = append(f.calls, "applyDNS")
	_ = journal.Record(JournalEntry{Kind: KindWindowsNRPT, Key: nrptRulePrefix})
	return nil
}

func (f *fakeWindowsOps) removeDNS(_ context.Context, _ winipcfg.LUID) error {
	f.calls = append(f.calls, "removeDNS")
	return nil
}

func (f *fakeWindowsOps) flushAdapter(_ winipcfg.LUID) {
	f.calls = append(f.calls, "flushAdapter")
}

func TestWindowsRouteFlushPrecedesIPv4AddressAssignment(t *testing.T) {
	dir := t.TempDir()
	jPath := filepath.Join(dir, "netcfg-journal.json")
	journal, err := LoadJournal(jPath)
	if err != nil {
		t.Fatalf("LoadJournal: %v", err)
	}

	d := Desired{
		Iface: "TunnelKit",
		Local: netip.MustParseAddr("10.212.0.11"),
		Peer:  netip.MustParseAddr("169.254.2.1"),
		MTU:   1400,
		Routes: []netip.Prefix{
			netip.MustParsePrefix("172.16.35.40/32"),
			netip.MustParsePrefix("172.16.10.1/32"),
		},
		Gateway: netip.MustParseAddr("198.51.100.10"),
		DNS: DNSPlan{
			Mode:    fortivpn.DNSSplit,
			Servers: nil, // Gateway returned ns [0.0.0.0, 0.0.0.0]
		},
	}
	snap := Snapshot{
		GatewayNextHop: netip.MustParseAddr("192.168.247.2"),
		GatewayLUID:    11,
	}

	ops := &fakeWindowsOps{}
	ap := &windowsApplied{
		luid:    18,
		ops:     ops,
		journal: journal,
		desired: d,
		snap:    snap,
	}

	steps := Plan(d, snap)
	if err := ap.executeSteps(context.Background(), steps); err != nil {
		t.Fatalf("executeSteps failed: %v", err)
	}

	flushIdx := slices.Index(ops.calls, "flushTunnelRoutes")
	setIPIdx := slices.Index(ops.calls, "setIPv4Address:10.212.0.11")
	addRoutesIdx := slices.Index(ops.calls, "addTunnelRoutes")
	if flushIdx < 0 || setIPIdx < 0 || addRoutesIdx < 0 {
		t.Fatalf("unexpected call sequence: %v", ops.calls)
	}
	if !(flushIdx < setIPIdx && setIPIdx < addRoutesIdx) {
		t.Fatalf("route flush must happen BEFORE setIPv4Address and addTunnelRoutes AFTER (got calls=%v)", ops.calls)
	}
	for i := setIPIdx + 1; i < len(ops.calls); i++ {
		if ops.calls[i] == "flushTunnelRoutes" {
			t.Fatalf("flushTunnelRoutes must never be called after setIPv4Address in the same pass: %v", ops.calls)
		}
	}
	if !slices.Contains(ops.calls, "disableIPv6") {
		t.Fatalf("expected disableIPv6 to be called: %v", ops.calls)
	}
	if !slices.Contains(ops.calls, "removeDNS") {
		t.Fatalf("expected removeDNS to be called when no VPN DNS servers are present: %v", ops.calls)
	}

	// Verify Update preserves the same safe order
	ops.calls = nil
	d2 := d
	d2.Routes = []netip.Prefix{netip.MustParsePrefix("172.16.150.11/32")}
	if err := ap.Update(context.Background(), d2); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	flushIdx = slices.Index(ops.calls, "flushTunnelRoutes")
	setIPIdx = slices.Index(ops.calls, "setIPv4Address:10.212.0.11")
	addRoutesIdx = slices.Index(ops.calls, "addTunnelRoutes")
	if !(flushIdx >= 0 && flushIdx < setIPIdx && setIPIdx < addRoutesIdx) {
		t.Fatalf("Update must flush old tunnel routes before setIPv4Address and addTunnelRoutes after (got calls=%v)", ops.calls)
	}

	if err := ap.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, err := os.Stat(jPath); !os.IsNotExist(err) {
		t.Errorf("expected journal file to be removed on Close, err = %v", err)
	}
}
