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

func TestDarwinApplyUpdateAndRollback(t *testing.T) {
	dir := t.TempDir()
	jPath := filepath.Join(dir, "netcfg-journal.json")

	type call struct {
		name  string
		args  string
		stdin string
	}
	var calls []call

	runner := func(_ context.Context, stdin string, name string, args ...string) (string, error) {
		argStr := strings.Join(args, " ")
		calls = append(calls, call{name: name, args: argStr, stdin: stdin})
		if name == "/sbin/route" && strings.HasPrefix(argStr, "-n get ") {
			return "   route to: 198.51.100.10\n    gateway: 192.168.1.1\n  interface: en0\n", nil
		}
		if name == "/usr/sbin/scutil" && strings.HasPrefix(stdin, "remove ") {
			return "  No such key\n", nil
		}
		return "", nil
	}

	d := Desired{
		Iface: "utun9",
		Local: netip.MustParseAddr("10.200.0.2"),
		Peer:  netip.MustParseAddr("169.254.2.1"),
		MTU:   1350,
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

	applied, err := applyDarwinWithRunner(context.Background(), d, jPath, runner)
	if err != nil {
		t.Fatalf("applyDarwinWithRunner failed: %v", err)
	}

	if _, err := os.Stat(jPath); err != nil {
		t.Fatalf("expected journal file to exist during active session: %v", err)
	}

	d2 := d
	d2.Routes = []netip.Prefix{
		netip.MustParsePrefix("10.10.0.0/16"),
	}
	if err := applied.Update(context.Background(), d2); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := os.Stat(jPath); !os.IsNotExist(err) {
		t.Errorf("expected journal file to be deleted after Close, err = %v", err)
	}

	var sawIfconfig, sawHostRouteAdd, sawScutilSet, sawScutilRemove, sawHostRouteDel bool
	for _, c := range calls {
		switch {
		case c.name == "/sbin/ifconfig" && strings.Contains(c.args, "utun9 inet 10.200.0.2"):
			sawIfconfig = true
		case c.name == "/sbin/route" && c.args == "-n add -host 198.51.100.10 192.168.1.1":
			sawHostRouteAdd = true
		case c.name == "/usr/sbin/scutil" && strings.Contains(c.stdin, "SearchOrder # 100000") && strings.Contains(c.stdin, "SupplementalMatchDomains * corp.example.com"):
			sawScutilSet = true
		case c.name == "/usr/sbin/scutil" && strings.Contains(c.stdin, "remove State:/Network/Service/TunnelKit/DNS"):
			sawScutilRemove = true
		case c.name == "/sbin/route" && c.args == "-n delete -host 198.51.100.10 192.168.1.1":
			sawHostRouteDel = true
		}
	}
	if !sawIfconfig || !sawHostRouteAdd || !sawScutilSet || !sawScutilRemove || !sawHostRouteDel {
		t.Fatalf("missing expected commands: ifconfig=%v hostAdd=%v scutilSet=%v scutilRem=%v hostDel=%v",
			sawIfconfig, sawHostRouteAdd, sawScutilSet, sawScutilRemove, sawHostRouteDel)
	}
}

func TestDarwinPreExistingHostRouteNotDeletedOnClose(t *testing.T) {
	dir := t.TempDir()
	jPath := filepath.Join(dir, "netcfg-journal.json")

	var hostRouteDeleted bool
	var sawOverridePrimary bool
	runner := func(_ context.Context, stdin string, name string, args ...string) (string, error) {
		argStr := strings.Join(args, " ")
		if name == "/sbin/route" && strings.HasPrefix(argStr, "-n get ") {
			return "   route to: 198.51.100.10\n    gateway: 192.168.1.1\n  interface: en0\n", nil
		}
		if name == "/sbin/route" && strings.HasPrefix(argStr, "-n add -host ") {
			return "route: writing to routing socket: File exists\nadd host 198.51.100.10: gateway 192.168.1.1: File exists\n", os.ErrExist
		}
		if name == "/sbin/route" && strings.HasPrefix(argStr, "-n delete -host ") {
			hostRouteDeleted = true
		}
		if name == "/usr/sbin/scutil" && strings.Contains(stdin, "OverridePrimary # 1") {
			sawOverridePrimary = true
		}
		return "", nil
	}

	d := Desired{
		Iface: "utun9",
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

	applied, err := applyDarwinWithRunner(context.Background(), d, jPath, runner)
	if err != nil {
		t.Fatalf("applyDarwinWithRunner failed: %v", err)
	}
	if !sawOverridePrimary {
		t.Fatal("expected DNSFull on macOS to set OverridePrimary # 1")
	}
	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if hostRouteDeleted {
		t.Fatal("expected pre-existing host route (File exists) NOT to be deleted on Close")
	}
}

func TestDarwinRecoveryRetainsJournalWhenSystemCommandFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	journal := NewJournal(path)
	if err := journal.Record(JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", NextHop: "192.0.2.1", Iface: "en0"}); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("system network operation unavailable")
	}
	if err := recoverDarwinWithRunner(context.Background(), path, run); err == nil {
		t.Fatal("expected recovery error")
	}
	remaining, err := LoadJournal(path)
	if err != nil || len(remaining.Entries()) != 1 {
		t.Fatalf("lost pending recovery: %v", err)
	}
}

type fakeDarwinRouteOps struct {
	snap       Snapshot
	calls      []string
	closed     bool
	sawSkipOld bool
}

func (f *fakeDarwinRouteOps) lookupRoute(_ context.Context, gw netip.Addr, _ string, skipExistingHostRoute bool) (Snapshot, error) {
	f.calls = append(f.calls, "lookup:"+gw.String())
	if skipExistingHostRoute {
		f.sawSkipOld = true
	}
	return f.snap, nil
}

func (f *fakeDarwinRouteOps) addHostRoute(_ context.Context, host, nextHop netip.Addr, iface string) (bool, error) {
	f.calls = append(f.calls, "addHost:"+host.String()+"@"+nextHop.String()+"/"+iface)
	return true, nil
}

func (f *fakeDarwinRouteOps) deleteHostRoute(_ context.Context, host, nextHop, iface string) error {
	f.calls = append(f.calls, "delHost:"+host+"@"+nextHop+"/"+iface)
	return nil
}

func (f *fakeDarwinRouteOps) addTunnelRoute(_ context.Context, pfx netip.Prefix, iface string) (bool, error) {
	f.calls = append(f.calls, "addTun:"+pfx.String()+"/"+iface)
	return true, nil
}

func (f *fakeDarwinRouteOps) deleteTunnelRoute(_ context.Context, pfx netip.Prefix, iface string) error {
	f.calls = append(f.calls, "delTun:"+pfx.String()+"/"+iface)
	return nil
}

func (f *fakeDarwinRouteOps) close() error {
	f.closed = true
	return nil
}

func TestDarwinNativeOpsApplyEnsureDNSAndClose(t *testing.T) {
	jPath := filepath.Join(t.TempDir(), "journal.json")
	routeOps := &fakeDarwinRouteOps{
		snap: Snapshot{GatewayNextHop: netip.MustParseAddr("192.168.1.1"), GatewayIface: "en0"},
	}

	dnsWiped := false
	scutilSetCount := 0
	runner := func(_ context.Context, stdin string, name string, _ ...string) (string, error) {
		if name == "/sbin/route" {
			t.Fatalf("native PF_ROUTE path must not shell out to /sbin/route")
		}
		if name == "/usr/sbin/scutil" {
			if strings.HasPrefix(stdin, "show ") {
				if dnsWiped {
					return "  No such key\n", nil
				}
				return "<dictionary> {\n  SearchOrder : 100000\n  OverridePrimary : 1\n  ServerAddresses : <array> { 0 : 10.0.0.53 }\n}\n", nil
			}
			if strings.Contains(stdin, "set State:/Network/Service/TunnelKit/DNS") {
				scutilSetCount++
			}
		}
		return "", nil
	}

	d := Desired{
		Iface:   "utun9",
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

	applied, err := applyDarwinWithOps(context.Background(), d, jPath, routeOps, runner)
	if err != nil {
		t.Fatalf("applyDarwinWithOps failed: %v", err)
	}
	ensurer := applied.(interface{ EnsureDNS(context.Context) error })
	if err := ensurer.EnsureDNS(context.Background()); err != nil {
		t.Fatalf("EnsureDNS when intact failed: %v", err)
	}
	if scutilSetCount != 1 {
		t.Fatalf("EnsureDNS should not rewrite intact DNS, setCount=%d", scutilSetCount)
	}

	dnsWiped = true
	if err := ensurer.EnsureDNS(context.Background()); err != nil {
		t.Fatalf("EnsureDNS restore failed: %v", err)
	}
	if scutilSetCount != 2 {
		t.Fatalf("EnsureDNS should restore wiped DNS, setCount=%d", scutilSetCount)
	}

	if err := applied.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !routeOps.closed {
		t.Fatal("expected Close to release native Darwin route socket")
	}
}
