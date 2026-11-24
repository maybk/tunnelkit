package netcfg

import (
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"path/filepath"
	"testing"

	"tunnelkit/fortivpn"
)

func TestLinuxDNSKeepsUnconfirmedChangesForRecovery(t *testing.T) {
	for _, failure := range []string{"domain", "dns", "resolvconf", "default-route"} {
		t.Run(failure, func(t *testing.T) {
			j := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
			unavailable := errors.New("temporarily unavailable")
			runner := func(_ context.Context, _ string, name string, args ...string) (string, error) {
				if failure == "resolvconf" && name == "resolvectl" {
					return "", exec.ErrNotFound
				}
				if name == "resolvconf" || args[0] == "revert" || args[0] == failure {
					return "", unavailable
				}
				return "", nil
			}
			if _, err := applyLinuxDNS(context.Background(), runner, "forti0", DNSPlan{Mode: fortivpn.DNSFull, Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")}}, j); err == nil {
				t.Fatal("expected DNS failure")
			}
			loaded, err := LoadJournal(j.path)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.Entries()) != 1 {
				t.Fatalf("expected exactly 1 recovery record for %s failure, got %d", failure, len(loaded.Entries()))
			}
			if err := loaded.Rollback(func(entry JournalEntry) error {
				return removeLinuxDNS(context.Background(), runner, entry.Iface, entry.Key)
			}); err == nil {
				t.Fatal("unconfirmed cleanup was treated as success")
			}
			if len(loaded.Entries()) == 0 {
				t.Fatal("failed cleanup removed recovery record")
			}
			if err := loaded.Rollback(func(JournalEntry) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLinuxDNSOnlyForgetsCommandsThatDidNotStart(t *testing.T) {
	j := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	runner := func(context.Context, string, string, ...string) (string, error) { return "", exec.ErrNotFound }
	_, err := applyLinuxDNS(context.Background(), runner, "forti0", DNSPlan{Mode: fortivpn.DNSFull, Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")}}, j)
	if err == nil {
		t.Fatal("expected missing DNS tools")
	}
	if len(j.Entries()) != 0 {
		t.Fatal("commands that never started must not leave changes to undo")
	}
}

func TestLinuxDNSCleanupRetainsTransientServiceFailures(t *testing.T) {
	runner := func(context.Context, string, string, ...string) (string, error) {
		return "Failed to get bus connection", errors.New("connection refused")
	}
	if err := removeLinuxDNS(context.Background(), runner, "forti0", "resolvectl"); err == nil {
		t.Fatal("service failure does not prove DNS settings were restored")
	}
}

func TestLinuxDNSUnavailableServiceFallsBackWithoutPendingMutation(t *testing.T) {
	j := NewJournal(filepath.Join(t.TempDir(), "journal.json"))
	runner := func(_ context.Context, _ string, name string, args ...string) (string, error) {
		if name == "resolvectl" {
			return "Failed to get bus connection", errors.New("service unavailable")
		}
		return "", nil
	}
	backend, err := applyLinuxDNS(context.Background(), runner, "forti0", DNSPlan{Mode: fortivpn.DNSFull, Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")}}, j)
	if err != nil || backend != linuxDNSBackendResolvconf {
		t.Fatal("unavailable service must fall back", err)
	}
	if err := j.Rollback(func(e JournalEntry) error { return removeLinuxDNS(context.Background(), runner, e.Iface, e.Key) }); err != nil {
		t.Fatal("unavailable service left a mutation that was never attempted", err)
	}
}
