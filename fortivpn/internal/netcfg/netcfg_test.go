package netcfg

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"tunnelkit/fortivpn"
)

func TestSanitizeRoutes(t *testing.T) {
	gw := netip.MustParseAddr("198.51.100.10")
	gatewayRoutes := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("10.10.1.5/16"),
		netip.MustParsePrefix("10.10.0.0/16"), // duplicate after Masked()
		netip.MustParsePrefix("127.0.0.0/8"),  // loopback - filtered
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("198.51.100.10/32"), // gateway /32 - filtered
		netip.MustParsePrefix("172.16.0.0/12"),
	}
	extra := []netip.Prefix{
		netip.MustParsePrefix("192.168.50.0/24"),
	}
	exclude := []netip.Prefix{
		netip.MustParsePrefix("172.16.0.0/12"),
	}

	got, _, err := SanitizeRoutes(fortivpn.RouteGateway, gatewayRoutes, extra, exclude, gw)
	if err != nil {
		t.Fatalf("SanitizeRoutes returned unexpected error: %v", err)
	}

	want := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/1"),
		netip.MustParsePrefix("10.10.0.0/16"),
		netip.MustParsePrefix("128.0.0.0/1"),
		netip.MustParsePrefix("192.168.50.0/24"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("route[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPlanAndSplitDNSDowngrade(t *testing.T) {
	gw := netip.MustParseAddr("198.51.100.10")
	routes := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/1"),
		netip.MustParsePrefix("128.0.0.0/1"),
	}
	d := Desired{
		Iface:   "utun7",
		Local:   netip.MustParseAddr("10.200.0.2"),
		Peer:    netip.MustParseAddr("169.254.2.1"),
		MTU:     1350,
		Routes:  routes,
		Gateway: gw,
		DNS: DNSPlan{
			Mode:    fortivpn.DNSSplit,
			Servers: []netip.Addr{netip.MustParseAddr("10.0.0.53")},
			Domains: nil,
		},
	}
	snap := Snapshot{
		GatewayNextHop: netip.MustParseAddr("192.168.1.1"),
		GatewayIface:   "en0",
	}

	if !SplitDNSNeedsDomains(d.DNS) {
		t.Fatalf("expected SplitDNSNeedsDomains to be true when domains are empty")
	}

	steps := Plan(d, snap)
	if len(steps) != 4 {
		t.Fatalf("got %d steps, want 4: %+v", len(steps), steps)
	}
	if steps[0].Kind != StepConfigureInterface {
		t.Errorf("step[0].Kind = %q, want %q", steps[0].Kind, StepConfigureInterface)
	}
	if steps[1].Kind != StepHostRoute || steps[1].Prefix != netip.MustParsePrefix("198.51.100.10/32") {
		t.Errorf("step[1] = %+v, want host_route for gateway", steps[1])
	}
}

func TestJournalPersistenceAndLIFO(t *testing.T) {
	dir := t.TempDir()
	jPath := filepath.Join(dir, "netcfg-journal.json")

	j := NewJournal(jPath)
	e1 := JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", NextHop: "192.168.1.1", Iface: "en0"}
	e2 := JournalEntry{Kind: KindDarwinDNSKey, Key: "State:/Network/Service/TunnelKit/DNS"}

	if err := j.Record(e1); err != nil {
		t.Fatalf("Record e1: %v", err)
	}
	if err := j.Record(e1); err != nil {
		t.Fatalf("Record duplicate e1: %v", err)
	}
	if err := j.Record(e2); err != nil {
		t.Fatalf("Record e2: %v", err)
	}

	loaded, err := LoadJournal(jPath)
	if err != nil {
		t.Fatalf("LoadJournal: %v", err)
	}
	entries := loaded.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0] != e2 || entries[1] != e1 {
		t.Errorf("got LIFO entries %+v, want [%+v, %+v]", entries, e2, e1)
	}

	if err := loaded.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(jPath); !os.IsNotExist(err) {
		t.Errorf("expected journal file to be removed, stat err = %v", err)
	}
}
