//go:build darwin

package netcfg

import (
	"context"
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

	applied, err := applyWithRunner(context.Background(), d, jPath, runner)
	if err != nil {
		t.Fatalf("applyWithRunner failed: %v", err)
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
		case c.name == "/usr/sbin/scutil" && strings.Contains(c.stdin, "SupplementalMatchDomains * corp.example.com"):
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

	applied, err := applyWithRunner(context.Background(), d, jPath, runner)
	if err != nil {
		t.Fatalf("applyWithRunner failed: %v", err)
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
