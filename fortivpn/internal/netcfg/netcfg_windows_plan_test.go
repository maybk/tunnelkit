package netcfg

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"tunnelkit/fortivpn"
)

type fakeWindowsOps struct {
	calls           []string
	hostRouteExists bool
	addTunnelErr    error
	deleteErr       error
	dnsCleanupErr   error
	nrptCleanupErr  error
	appliedRoutes   []netip.Prefix
	lookupSnap      Snapshot
	lookupErr       error
}

func (f *fakeWindowsOps) lookupGatewayRoute(gateway netip.Addr, _ uint64) (Snapshot, error) {
	f.calls = append(f.calls, "lookupGatewayRoute:"+gateway.String())
	if f.lookupErr != nil {
		return Snapshot{}, f.lookupErr
	}
	return f.lookupSnap, nil
}

func (f *fakeWindowsOps) disableIPv6(_ uint64) {
	f.calls = append(f.calls, "disableIPv6")
}

func (f *fakeWindowsOps) flushTunnelRoutes(_ uint64) error {
	f.calls = append(f.calls, "flushTunnelRoutes")
	f.appliedRoutes = nil
	return nil
}

func (f *fakeWindowsOps) configureIPv4Interface(_ uint64, _ int) error {
	f.calls = append(f.calls, "configureIPv4Interface")
	return nil
}

func (f *fakeWindowsOps) setIPv4Address(_ uint64, local netip.Addr) error {
	f.calls = append(f.calls, "setIPv4Address:"+local.String())
	return nil
}

func (f *fakeWindowsOps) addTunnelRoutes(_ uint64, routes []netip.Prefix) error {
	f.calls = append(f.calls, "addTunnelRoutes")
	if f.addTunnelErr != nil {
		return f.addTunnelErr
	}
	f.appliedRoutes = append([]netip.Prefix(nil), routes...)
	return nil
}

func (f *fakeWindowsOps) addHostRoute(_ uint64, prefix netip.Prefix, _ netip.Addr) (bool, error) {
	f.calls = append(f.calls, "addHostRoute:"+prefix.String())
	if f.hostRouteExists {
		return false, nil
	}
	return true, nil
}

func (f *fakeWindowsOps) deleteHostRoute(_ uint64, prefix netip.Prefix, _ netip.Addr) error {
	f.calls = append(f.calls, "deleteHostRoute:"+prefix.String())
	return f.deleteErr
}

func (f *fakeWindowsOps) applyDNS(_ context.Context, _ uint64, _ DNSPlan, journal *Journal) error {
	f.calls = append(f.calls, "applyDNS")
	_ = journal.Record(JournalEntry{Kind: KindWindowsNRPT, Key: nrptRulePrefix})
	return nil
}

func (f *fakeWindowsOps) removeDNS(_ context.Context, _ uint64) error {
	f.calls = append(f.calls, "removeDNS")
	return f.dnsCleanupErr
}

func (f *fakeWindowsOps) clearNRPT(_ context.Context) error {
	f.calls = append(f.calls, "clearNRPT")
	return f.nrptCleanupErr
}

func (f *fakeWindowsOps) flushAdapter(_ uint64) {
	f.calls = append(f.calls, "flushAdapter")
}

func TestWindowsDNSOffRetainsFailedCleanupForRetry(t *testing.T) {
	journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	entry := JournalEntry{Kind: KindWindowsNRPT, Key: nrptRulePrefix}
	if err := journal.Record(entry); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("DNS cleanup failed")
	ops := &fakeWindowsOps{dnsCleanupErr: failure}
	previous := Desired{Iface: "TunnelKit", Local: netip.MustParseAddr("10.0.0.10"), DNS: DNSPlan{Mode: fortivpn.DNSFull}}
	a := &windowsApplied{luid: 18, journal: journal, ops: ops, desired: previous}
	next := previous
	next.DNS.Mode = fortivpn.DNSOff
	if err := a.Update(context.Background(), next); !errors.Is(err, failure) {
		t.Fatalf("lost DNS cleanup error: %v", err)
	}
	loaded, err := LoadJournal(journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries()) != 1 || a.desired.DNS.Mode != previous.DNS.Mode {
		t.Fatal("lost pending DNS cleanup or committed failed update")
	}
	ops.dnsCleanupErr = nil
	if err := a.Update(context.Background(), next); err != nil || len(journal.Entries()) != 0 {
		t.Fatalf("DNS cleanup retry failed: %v", err)
	}
}

func TestWindowsHostRouteCleanupFailureRetainsJournal(t *testing.T) {
	journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	entry := JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", LUID: 11}
	if err := journal.Record(entry); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("route cleanup failed")
	ops := &fakeWindowsOps{deleteErr: failure}
	previous := Desired{Gateway: netip.MustParseAddr(entry.Gateway), Routes: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}}
	a := &windowsApplied{journal: journal, desired: previous, ops: ops}
	next := previous
	next.Routes = nil
	if err := a.Update(context.Background(), next); !errors.Is(err, failure) {
		t.Fatalf("lost host route cleanup error: %v", err)
	}
	loaded, err := LoadJournal(journal.path)
	if err != nil || len(loaded.Entries()) != 1 || len(a.desired.Routes) == 0 {
		t.Fatal("host route cleanup failure lost retry state", err)
	}
	ops.deleteErr = nil
	if err := a.Update(context.Background(), next); err != nil || len(journal.Entries()) != 0 {
		t.Fatalf("host route cleanup retry failed: %v", err)
	}
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

func TestWindowsCloseRetainsPendingRecoveryOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	journal := NewJournal(path)
	if err := journal.Record(JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", NextHop: "192.0.2.1", LUID: 123}); err != nil {
		t.Fatal(err)
	}
	applied := &windowsApplied{journal: journal, ops: &fakeWindowsOps{deleteErr: errors.New("system operation unavailable")}}
	if err := applied.Close(context.Background()); err == nil {
		t.Fatal("expected cleanup error")
	}
	pending, err := LoadJournal(path)
	if err != nil || len(pending.Entries()) != 1 {
		t.Fatalf("lost pending recovery: %v", err)
	}
}

func TestWindowsNRPTRollbackFailureRetainsJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	journal := NewJournal(path)
	if err := journal.Record(JournalEntry{Kind: KindWindowsNRPT, Key: nrptRulePrefix}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("registry busy")
	ops := &fakeWindowsOps{nrptCleanupErr: failure}
	if err := rollbackWindowsJournalWithOps(context.Background(), journal, ops); !errors.Is(err, failure) {
		t.Fatalf("expected NRPT rollback error, got %v", err)
	}
	if !slices.Contains(ops.calls, "clearNRPT") {
		t.Fatalf("expected rollback to invoke ops.clearNRPT, got calls=%v", ops.calls)
	}
	pending, err := LoadJournal(path)
	if err != nil || len(pending.Entries()) != 1 {
		t.Fatalf("NRPT rollback failure must retain pending journal entry: err=%v entries=%v", err, pending.Entries())
	}

	ops.nrptCleanupErr = nil
	if err := rollbackWindowsJournalWithOps(context.Background(), pending, ops); err != nil {
		t.Fatalf("NRPT rollback retry failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected journal deleted after successful NRPT rollback, stat err=%v", err)
	}
}

func TestWindowsGatewayChangeUpdatesHostRouteAndSnapshot(t *testing.T) {
	journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	oldGW := netip.MustParseAddr("198.51.100.10")
	newGW := netip.MustParseAddr("198.51.100.20")
	if err := journal.Record(buildHostRouteEntry(oldGW, netip.MustParseAddr("192.168.1.1"), "", 11)); err != nil {
		t.Fatal(err)
	}
	previous := NormalizeDesired(Desired{
		Iface:   "TunnelKit",
		Local:   netip.MustParseAddr("10.0.0.2"),
		Gateway: oldGW,
		Routes: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
	})
	ops := &fakeWindowsOps{
		lookupSnap: Snapshot{
			GatewayNextHop: netip.MustParseAddr("192.168.2.1"),
			GatewayLUID:    22,
		},
	}
	a := &windowsApplied{
		luid:    18,
		ops:     ops,
		journal: journal,
		desired: previous,
		snap: Snapshot{
			GatewayNextHop: netip.MustParseAddr("192.168.1.1"),
			GatewayLUID:    11,
		},
	}
	next := previous
	next.Gateway = newGW
	if err := a.Update(context.Background(), next); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !slices.Contains(ops.calls, "deleteHostRoute:198.51.100.10/32") ||
		!slices.Contains(ops.calls, "lookupGatewayRoute:198.51.100.20") ||
		!slices.Contains(ops.calls, "addHostRoute:198.51.100.20/32") {
		t.Fatalf("unexpected calls on gateway change: %v", ops.calls)
	}
	entries := journal.Entries()
	if len(entries) != 1 || entries[0].Gateway != newGW.String() || entries[0].LUID != 22 {
		t.Fatalf("unexpected journal entries after gateway change: %+v", entries)
	}
}

func TestWindowsApplyRollsBackOnFailure(t *testing.T) {
	jPath := filepath.Join(t.TempDir(), "journal.json")
	ops := &fakeWindowsOps{
		lookupSnap: Snapshot{
			GatewayNextHop: netip.MustParseAddr("192.168.1.1"),
			GatewayLUID:    11,
		},
		addTunnelErr: errors.New("route table busy"),
	}
	d := Desired{
		Iface:   "TunnelKit",
		Local:   netip.MustParseAddr("10.212.0.12"),
		MTU:     1400,
		Gateway: netip.MustParseAddr("198.51.100.10"),
		Routes: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
	}

	if _, err := applyWindowsWithOps(context.Background(), d, jPath, 18, ops); err == nil {
		t.Fatal("expected applyWindowsWithOps to fail when adding tunnel routes fails")
	}
	if !slices.Contains(ops.calls, "addHostRoute:198.51.100.10/32") ||
		!slices.Contains(ops.calls, "flushAdapter") ||
		!slices.Contains(ops.calls, "deleteHostRoute:198.51.100.10/32") {
		t.Fatalf("expected failed apply to roll back host route and flush adapter, got calls=%v", ops.calls)
	}
	if _, err := os.Stat(jPath); !os.IsNotExist(err) {
		t.Fatalf("expected journal cleaned after rollback, stat err=%v", err)
	}
}
