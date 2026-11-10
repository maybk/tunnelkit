//go:build darwin || linux

package netcfg

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"testing"

	"tunnelkit/fortivpn"
)

func TestDNSBackendSwitchRetainsFailedCleanupForRetry(t *testing.T) {
	for _, oldBackend := range []string{linuxDNSBackendResolvectl, linuxDNSBackendResolvconf} {
		t.Run(oldBackend, func(t *testing.T) {
			newBackend := linuxDNSBackendResolvconf
			if oldBackend == newBackend {
				newBackend = linuxDNSBackendResolvectl
			}
			journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
			old := JournalEntry{Kind: KindLinuxDNS, Iface: "forti0", Key: oldBackend}
			if err := journal.Record(old); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("DNS service unavailable")
			cleanupErr := failure
			desired := NormalizeDesired(Desired{Iface: "forti0", DNS: DNSPlan{
				Mode: fortivpn.DNSFull, Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")},
			}})
			a := &unixApplied{journal: journal, desired: desired, ops: unixPlatformOps{
				dnsJournalKind: KindLinuxDNS,
				applyDNS: func(_ context.Context, _ cmdRunner, iface string, _ DNSPlan, j *Journal) (string, error) {
					return newBackend, j.Record(JournalEntry{Kind: KindLinuxDNS, Iface: iface, Key: newBackend})
				},
				removeDNS: func(context.Context, cmdRunner, string, string) error { return cleanupErr },
			}}
			if err := a.Update(context.Background(), desired); !errors.Is(err, failure) {
				t.Fatalf("lost cleanup error: %v", err)
			}
			loaded, err := LoadJournal(journal.path)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.Entries()) != 2 {
				t.Fatalf("lost retry state: entries=%v err=%v", loaded.Entries(), err)
			}
			cleanupErr = nil
			if err := a.Update(context.Background(), desired); err != nil {
				t.Fatal(err)
			}
			entries := journal.Entries()
			if len(entries) != 1 || entries[0].Key != newBackend {
				t.Fatalf("unexpected state after retry: %v", entries)
			}
			if err := a.Close(context.Background()); err != nil || len(journal.Entries()) != 0 {
				t.Fatalf("cleanup incomplete: %v", err)
			}
		})
	}
}

func TestDNSOffAfterFailedBackendSwitchClearsBothBackends(t *testing.T) {
	journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	for _, backend := range []string{linuxDNSBackendResolvectl, linuxDNSBackendResolvconf} {
		if err := journal.Record(JournalEntry{Kind: KindLinuxDNS, Iface: "forti0", Key: backend}); err != nil {
			t.Fatal(err)
		}
	}
	var cleaned []string
	desired := NormalizeDesired(Desired{Iface: "forti0", DNS: DNSPlan{Mode: fortivpn.DNSOff}})
	a := &unixApplied{journal: journal, desired: desired, ops: unixPlatformOps{
		dnsJournalKind: KindLinuxDNS,
		removeDNS: func(_ context.Context, _ cmdRunner, _ string, backend string) error {
			cleaned = append(cleaned, backend)
			return nil
		},
	}}
	if err := a.Update(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if len(cleaned) != 2 || len(journal.Entries()) != 0 {
		t.Fatalf("DNSOff left pending DNS: cleaned=%v entries=%v", cleaned, journal.Entries())
	}
}

func TestDNSCleanupRetainsJournalOnPersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	journal := NewJournal(path)
	entry := JournalEntry{Kind: KindLinuxDNS, Iface: "forti0", Key: linuxDNSBackendResolvectl}
	if err := journal.Record(entry); err != nil {
		t.Fatal(err)
	}
	a := &unixApplied{journal: journal, ops: unixPlatformOps{
		dnsJournalKind: KindLinuxDNS,
		removeDNS:      func(context.Context, cmdRunner, string, string) error { return nil },
	}}
	// Replacing an existing directory with a journal file must fail on all platforms.
	journal.path = t.TempDir()
	if err := a.cleanupDNS(context.Background(), JournalEntry{}); err == nil {
		t.Fatal("expected journal persistence failure")
	}
	loaded, err := LoadJournal(path)
	if err != nil || len(loaded.Entries()) != 1 || len(journal.Entries()) != 1 {
		t.Fatal("failed persistence discarded retry state", err)
	}
	journal.path = path
	if err := a.cleanupDNS(context.Background(), JournalEntry{}); err != nil || len(journal.Entries()) != 0 {
		t.Fatalf("journal cleanup retry failed: %v", err)
	}
}

func TestHostRouteCleanupFailureRetainsJournal(t *testing.T) {
	journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	entry := JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", Iface: "en0"}
	if err := journal.Record(entry); err != nil {
		t.Fatal(err)
	}
	previous := NormalizeDesired(Desired{Gateway: netip.MustParseAddr(entry.Gateway), Routes: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}})
	failure := errors.New("route cleanup failed")
	cleanupErr := failure
	a := &unixApplied{journal: journal, desired: previous, ops: unixPlatformOps{
		deleteHostRoute: func(context.Context, cmdRunner, string, string, string) error { return cleanupErr },
	}}
	next := previous
	next.Routes = nil
	if err := a.Update(context.Background(), next); !errors.Is(err, failure) {
		t.Fatalf("lost host route cleanup error: %v", err)
	}
	loaded, err := LoadJournal(journal.path)
	if err != nil || len(loaded.Entries()) != 1 || len(a.desired.Routes) == 0 {
		t.Fatal("host route cleanup failure lost retry state", err)
	}
	cleanupErr = nil
	if err := a.Update(context.Background(), next); err != nil || len(journal.Entries()) != 0 {
		t.Fatalf("host route cleanup retry failed: %v", err)
	}
}

func TestUnixGatewayChangeUpdatesHostRouteAndSnapshot(t *testing.T) {
	journal := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	oldGW := netip.MustParseAddr("198.51.100.10")
	newGW := netip.MustParseAddr("198.51.100.20")
	if err := journal.Record(buildHostRouteEntry(oldGW, netip.MustParseAddr("192.168.1.1"), "en0", 0)); err != nil {
		t.Fatal(err)
	}
	previous := NormalizeDesired(Desired{
		Iface:   "utun7",
		Local:   netip.MustParseAddr("10.0.0.2"),
		Gateway: oldGW,
		Routes: []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/1"),
			netip.MustParsePrefix("128.0.0.0/1"),
		},
	})
	var deletedHost, addedHost, addedIface string
	a := &unixApplied{
		journal: journal,
		desired: previous,
		snap:    Snapshot{GatewayNextHop: netip.MustParseAddr("192.168.1.1"), GatewayIface: "en0"},
		ops: unixPlatformOps{
			lookupRoute: func(_ context.Context, _ cmdRunner, gw netip.Addr, _ string) (Snapshot, error) {
				if gw != newGW {
					t.Fatalf("unexpected lookup gateway: %s", gw)
				}
				return Snapshot{GatewayNextHop: netip.MustParseAddr("192.168.2.1"), GatewayIface: "en1"}, nil
			},
			deleteHostRoute: func(_ context.Context, _ cmdRunner, host, _, _ string) error {
				deletedHost = host
				return nil
			},
			addHostRoute: func(_ context.Context, _ cmdRunner, host, _ netip.Addr, iface string) (bool, error) {
				addedHost = host.String()
				addedIface = iface
				return true, nil
			},
			addTunnelRoute: func(context.Context, cmdRunner, netip.Prefix, string) (bool, error) {
				return true, nil
			},
		},
	}

	next := previous
	next.Gateway = newGW
	if err := a.Update(context.Background(), next); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if deletedHost != oldGW.String() || addedHost != newGW.String() || addedIface != "en1" {
		t.Fatalf("unexpected gateway host route update: deleted=%q added=%q iface=%q", deletedHost, addedHost, addedIface)
	}
	entries := journal.Entries()
	if len(entries) != 1 || entries[0].Gateway != newGW.String() || entries[0].Iface != "en1" {
		t.Fatalf("unexpected journal entries after gateway change: %+v", entries)
	}
}
