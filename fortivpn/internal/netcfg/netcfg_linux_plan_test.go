package netcfg

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tunnelkit/fortivpn"
)

func TestLinuxApplyUpdateAndRollbackWithResolvectl(t *testing.T) {
	dir := t.TempDir()
	jPath := filepath.Join(dir, "netcfg-linux-journal.json")

	type call struct {
		name  string
		args  string
		stdin string
	}
	var calls []call

	runner := func(_ context.Context, stdin string, name string, args ...string) (string, error) {
		argStr := strings.Join(args, " ")
		calls = append(calls, call{name: name, args: argStr, stdin: stdin})
		if name == "ip" && argStr == "-4 route get 198.51.100.10" {
			return "198.51.100.10 via 192.168.1.1 dev eth0 src 192.168.1.50 uid 0\n    cache\n", nil
		}
		return "", nil
	}

	d := Desired{
		Iface: "forti0",
		Local: netip.MustParseAddr("10.200.0.2"),
		Peer:  netip.MustParseAddr("169.254.2.1"),
		MTU:   1400,
		Routes: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
		Gateway: netip.MustParseAddr("198.51.100.10"),
		DNS: DNSPlan{
			Mode:    fortivpn.DNSSplit,
			Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")},
			Domains: []string{"corp.example.com"},
		},
	}

	applied, err := applyLinuxWithRunner(context.Background(), d, jPath, runner)
	if err != nil {
		t.Fatalf("applyLinuxWithRunner failed: %v", err)
	}

	if _, err := os.Stat(jPath); err != nil {
		t.Fatalf("expected journal file to exist during active session: %v", err)
	}

	d2 := d
	d2.Routes = []netip.Prefix{
		netip.MustParsePrefix("10.10.0.0/16"),
	}
	d2.DNS.Mode = fortivpn.DNSFull
	if err := applied.Update(context.Background(), d2); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := os.Stat(jPath); !os.IsNotExist(err) {
		t.Errorf("expected journal file to be deleted after Close, err = %v", err)
	}

	var sawAddrFlush, sawAddr, sawLinkUp, sawTunnelAdd, sawHostRouteAdd, sawResolvectlSplit, sawResolvectlFull, sawResolvectlRevert, sawHostRouteDel bool
	for _, c := range calls {
		switch {
		case c.name == "ip" && c.args == "-4 addr flush dev forti0":
			sawAddrFlush = true
		case c.name == "ip" && c.args == "-4 addr replace 10.200.0.2/32 peer 169.254.2.1 dev forti0":
			sawAddr = true
		case c.name == "ip" && c.args == "link set dev forti0 mtu 1400 up":
			sawLinkUp = true
		case c.name == "ip" && c.args == "-4 route add 0.0.0.0/1 dev forti0":
			sawTunnelAdd = true
		case c.name == "ip" && c.args == "-4 route add 198.51.100.10/32 via 192.168.1.1 dev eth0":
			sawHostRouteAdd = true
		case c.name == "resolvectl" && c.args == "domain forti0 ~corp.example.com":
			sawResolvectlSplit = true
		case c.name == "resolvectl" && c.args == "domain forti0 ~. corp.example.com":
			sawResolvectlFull = true
		case c.name == "resolvectl" && c.args == "revert forti0":
			sawResolvectlRevert = true
		case c.name == "ip" && c.args == "-4 route del 198.51.100.10/32 via 192.168.1.1 dev eth0":
			sawHostRouteDel = true
		}
	}
	if !sawAddrFlush || !sawAddr || !sawLinkUp || !sawTunnelAdd || !sawHostRouteAdd || !sawResolvectlSplit || !sawResolvectlFull || !sawResolvectlRevert || !sawHostRouteDel {
		t.Fatalf("missing expected Linux commands: flush=%v addr=%v link=%v tunnelAdd=%v hostAdd=%v split=%v full=%v revert=%v hostDel=%v",
			sawAddrFlush, sawAddr, sawLinkUp, sawTunnelAdd, sawHostRouteAdd, sawResolvectlSplit, sawResolvectlFull, sawResolvectlRevert, sawHostRouteDel)
	}
}

func TestLinuxPreExistingHostRouteAndResolvconfFallback(t *testing.T) {
	dir := t.TempDir()
	jPath := filepath.Join(dir, "netcfg-linux-journal.json")

	var hostRouteDeleted bool
	var sawResolvconfAdd, sawResolvconfDel bool
	runner := func(_ context.Context, stdin string, name string, args ...string) (string, error) {
		argStr := strings.Join(args, " ")
		if name == "ip" && argStr == "-4 route get 198.51.100.10" {
			return "198.51.100.10 via 192.168.1.1 dev eth0 src 192.168.1.50\n", nil
		}
		if name == "ip" && strings.HasPrefix(argStr, "-4 route add 198.51.100.10/32") {
			return "RTNETLINK answers: File exists\n", os.ErrExist
		}
		if name == "ip" && strings.HasPrefix(argStr, "-4 route del 198.51.100.10/32") {
			hostRouteDeleted = true
		}
		if name == "resolvectl" {
			return "Failed to get bus connection", os.ErrNotExist
		}
		if name == "resolvconf" && argStr == "-a forti0.tunnelkit" && strings.Contains(stdin, "nameserver 10.0.0.53") {
			sawResolvconfAdd = true
		}
		if name == "resolvconf" && argStr == "-d forti0.tunnelkit" {
			sawResolvconfDel = true
		}
		return "", nil
	}

	d := Desired{
		Iface: "forti0",
		Local: netip.MustParseAddr("10.200.0.2"),
		Peer:  netip.MustParseAddr("169.254.2.1"),
		MTU:   1400,
		Routes: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
		Gateway: netip.MustParseAddr("198.51.100.10"),
		DNS: DNSPlan{
			Mode:    fortivpn.DNSFull,
			Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")},
			Domains: []string{"corp.example.com"},
		},
	}

	applied, err := applyLinuxWithRunner(context.Background(), d, jPath, runner)
	if err != nil {
		t.Fatalf("applyLinuxWithRunner failed: %v", err)
	}
	if !sawResolvconfAdd {
		t.Fatal("expected fallback to resolvconf -a when resolvectl fails")
	}
	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !sawResolvconfDel {
		t.Fatal("expected resolvconf -d on Close")
	}
	if hostRouteDeleted {
		t.Fatal("expected pre-existing Linux host route (File exists) NOT to be deleted on Close")
	}
}

func TestLinuxRecoveryRetainsJournalWhenSystemCommandFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	journal := NewJournal(path)
	if err := journal.Record(JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", NextHop: "192.0.2.1", Iface: "en0"}); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("system network operation unavailable")
	}
	if err := recoverLinuxWithRunner(context.Background(), path, run); err == nil {
		t.Fatal("expected recovery error")
	}
	remaining, err := LoadJournal(path)
	if err != nil || len(remaining.Entries()) != 1 {
		t.Fatalf("lost pending recovery: %v", err)
	}
}

func TestLinuxDNSOffUpdatePreservesFailedCleanupForRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	failCleanup := false
	runner := func(_ context.Context, _ string, name string, args ...string) (string, error) {
		if failCleanup && name == "resolvectl" && args[0] == "revert" {
			return "", errors.New("resolver temporarily unavailable")
		}
		return "", nil
	}
	d := Desired{Iface: "forti0", Local: netip.MustParseAddr("10.200.0.2"), Peer: netip.MustParseAddr("169.254.2.1"), MTU: 1400, DNS: DNSPlan{Mode: fortivpn.DNSFull, Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")}}}
	applied, err := applyLinuxWithRunner(context.Background(), d, path, runner)
	if err != nil {
		t.Fatal(err)
	}
	failCleanup = true
	d.DNS.Mode = fortivpn.DNSOff
	if err := applied.Update(context.Background(), d); err == nil {
		t.Fatal("failed DNS cleanup must remain retryable")
	}
	j, err := LoadJournal(path)
	if err != nil || len(j.Entries()) != 1 {
		t.Fatal("failed cleanup lost its record", err)
	}
	failCleanup = false
	if err := applied.Update(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if err := applied.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("successful cleanup left its journal", err)
	}
}

type fakeLinuxNetlinkOps struct {
	snap       Snapshot
	calls      []string
	sawSkipOld bool
}

func (f *fakeLinuxNetlinkOps) configureInterface(_ context.Context, iface string, local, _ netip.Addr, _ int) error {
	f.calls = append(f.calls, "iface:"+iface+"="+local.String())
	return nil
}

func (f *fakeLinuxNetlinkOps) lookupRoute(_ context.Context, gw netip.Addr, _ string, skipExistingHostRoute bool) (Snapshot, error) {
	f.calls = append(f.calls, "lookup:"+gw.String())
	if skipExistingHostRoute {
		f.sawSkipOld = true
	}
	return f.snap, nil
}

func (f *fakeLinuxNetlinkOps) addHostRoute(_ context.Context, host, nextHop netip.Addr, iface string) (bool, error) {
	f.calls = append(f.calls, "addHost:"+host.String()+"@"+nextHop.String()+"/"+iface)
	return true, nil
}

func (f *fakeLinuxNetlinkOps) deleteHostRoute(_ context.Context, host, nextHop, iface string) error {
	f.calls = append(f.calls, "delHost:"+host+"@"+nextHop+"/"+iface)
	return nil
}

func (f *fakeLinuxNetlinkOps) addTunnelRoute(_ context.Context, pfx netip.Prefix, iface string) (bool, error) {
	f.calls = append(f.calls, "addTun:"+pfx.String()+"/"+iface)
	return true, nil
}

func (f *fakeLinuxNetlinkOps) deleteTunnelRoute(_ context.Context, pfx netip.Prefix, iface string) error {
	f.calls = append(f.calls, "delTun:"+pfx.String()+"/"+iface)
	return nil
}

func (f *fakeLinuxNetlinkOps) close() error { return nil }

func TestLinuxNativeNetlinkOpsApplyEnsureDNSAndClose(t *testing.T) {
	jPath := filepath.Join(t.TempDir(), "journal.json")
	nlOps := &fakeLinuxNetlinkOps{
		snap: Snapshot{GatewayNextHop: netip.MustParseAddr("192.168.1.1"), GatewayIface: "eth0"},
	}

	dnsWiped := false
	dnsSetCount := 0
	runner := func(_ context.Context, _ string, name string, args ...string) (string, error) {
		if name == "ip" {
			t.Fatalf("native Netlink path must not shell out to ip command")
		}
		if name == "resolvectl" && len(args) > 0 {
			if args[0] == "status" {
				if dnsWiped {
					return "Link 7 (forti0)\n  Current Scopes: none\n", nil
				}
				return "Link 7 (forti0)\n  DNS Servers: 10.0.0.53\n  DNS Domain: ~.\n", nil
			}
			if args[0] == "dns" {
				dnsSetCount++
			}
		}
		return "", nil
	}

	d := Desired{
		Iface:   "forti0",
		Local:   netip.MustParseAddr("10.200.0.2"),
		Peer:    netip.MustParseAddr("169.254.2.1"),
		MTU:     1400,
		Gateway: netip.MustParseAddr("198.51.100.10"),
		Routes:  []netip.Prefix{netip.MustParsePrefix("0.0.0.0/1"), netip.MustParsePrefix("128.0.0.0/1")},
		DNS: DNSPlan{
			Mode:    fortivpn.DNSFull,
			Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")},
		},
	}

	applied, err := applyLinuxWithOps(context.Background(), d, jPath, nlOps, runner)
	if err != nil {
		t.Fatalf("applyLinuxWithOps failed: %v", err)
	}
	ensurer := applied.(interface{ EnsureDNS(context.Context) error })
	if err := ensurer.EnsureDNS(context.Background()); err != nil {
		t.Fatalf("EnsureDNS when intact failed: %v", err)
	}
	if dnsSetCount != 1 {
		t.Fatalf("EnsureDNS should not rewrite intact resolvectl config, dnsSetCount=%d", dnsSetCount)
	}

	dnsWiped = true
	if err := ensurer.EnsureDNS(context.Background()); err != nil {
		t.Fatalf("EnsureDNS restore failed: %v", err)
	}
	if dnsSetCount != 2 {
		t.Fatalf("EnsureDNS should restore wiped resolvectl config, dnsSetCount=%d", dnsSetCount)
	}

	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestLinuxEnsureDNSBackendFallbackUpdatesJournalAndCleansUp(t *testing.T) {
	jPath := filepath.Join(t.TempDir(), "journal.json")
	nlOps := &fakeLinuxNetlinkOps{
		snap: Snapshot{GatewayNextHop: netip.MustParseAddr("192.168.1.1"), GatewayIface: "eth0"},
	}

	resolvectlDown := false
	var sawResolvectlRevert, sawResolvconfAdd, sawResolvconfDel bool
	runner := func(_ context.Context, _ string, name string, args ...string) (string, error) {
		argStr := strings.Join(args, " ")
		if name == "resolvectl" {
			if len(args) > 0 && args[0] == "revert" {
				sawResolvectlRevert = true
				return "", nil
			}
			if resolvectlDown {
				return "Failed to get bus connection", os.ErrNotExist
			}
			return "Link 7 (forti0)\n  DNS Servers: 10.0.0.53\n  DNS Domain: ~.\n", nil
		}
		if name == "resolvconf" && argStr == "-a forti0.tunnelkit" {
			sawResolvconfAdd = true
		}
		if name == "resolvconf" && argStr == "-d forti0.tunnelkit" {
			sawResolvconfDel = true
		}
		return "", nil
	}

	d := Desired{
		Iface: "forti0",
		Local: netip.MustParseAddr("10.200.0.2"),
		Peer:  netip.MustParseAddr("169.254.2.1"),
		MTU:   1400,
		DNS: DNSPlan{
			Mode:    fortivpn.DNSFull,
			Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")},
		},
	}

	applied, err := applyLinuxWithOps(context.Background(), d, jPath, nlOps, runner)
	if err != nil {
		t.Fatalf("applyLinuxWithOps failed: %v", err)
	}

	// Simulate systemd-resolved going away after wake so EnsureDNS falls back to resolvconf.
	resolvectlDown = true
	ensurer := applied.(interface{ EnsureDNS(context.Context) error })
	if err := ensurer.EnsureDNS(context.Background()); err != nil {
		t.Fatalf("EnsureDNS fallback failed: %v", err)
	}
	if !sawResolvconfAdd || !sawResolvectlRevert {
		t.Fatalf("expected fallback to add resolvconf and clean up resolvectl: add=%v revert=%v", sawResolvconfAdd, sawResolvectlRevert)
	}
	loaded, err := LoadJournal(jPath)
	if err != nil {
		t.Fatalf("LoadJournal failed: %v", err)
	}
	entries := loaded.Entries()
	if len(entries) != 1 || entries[0].Key != linuxDNSBackendResolvconf {
		t.Fatalf("expected journal to track active resolvconf backend, got %+v", entries)
	}

	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !sawResolvconfDel {
		t.Fatal("expected Close to remove fallback resolvconf entry")
	}
}
