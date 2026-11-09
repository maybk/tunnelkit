//go:build linux

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

	var sawAddrFlush, sawAddr, sawLinkUp, sawTunnelReplace, sawHostRouteAdd, sawResolvectlSplit, sawResolvectlFull, sawResolvectlRevert, sawHostRouteDel bool
	for _, c := range calls {
		switch {
		case c.name == "ip" && c.args == "-4 addr flush dev forti0":
			sawAddrFlush = true
		case c.name == "ip" && c.args == "-4 addr replace 10.200.0.2/32 peer 169.254.2.1 dev forti0":
			sawAddr = true
		case c.name == "ip" && c.args == "link set dev forti0 mtu 1400 up":
			sawLinkUp = true
		case c.name == "ip" && c.args == "-4 route replace 0.0.0.0/1 dev forti0":
			sawTunnelReplace = true
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
	if !sawAddrFlush || !sawAddr || !sawLinkUp || !sawTunnelReplace || !sawHostRouteAdd || !sawResolvectlSplit || !sawResolvectlFull || !sawResolvectlRevert || !sawHostRouteDel {
		t.Fatalf("missing expected Linux commands: flush=%v addr=%v link=%v replace=%v hostAdd=%v split=%v full=%v revert=%v hostDel=%v",
			sawAddrFlush, sawAddr, sawLinkUp, sawTunnelReplace, sawHostRouteAdd, sawResolvectlSplit, sawResolvectlFull, sawResolvectlRevert, sawHostRouteDel)
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

func TestRecoveryRetainsJournalWhenSystemCommandFails(t *testing.T) {
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
