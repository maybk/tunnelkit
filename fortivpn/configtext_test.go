package fortivpn

import (
	"strings"
	"testing"
	"time"
)

func TestParseAndFormatConfigTextRoundTrip(t *testing.T) {
	digest := strings.Repeat("b", 64)
	input := strings.Join([]string{
		"# Sample config",
		"host = 198.51.100.10:10443",
		"username = alice",
		"password = s3cr=t",
		"realm = corp",
		"trusted-cert = " + digest,
		"set-dns = 0",
		"pppd-use-peerdns = 0",
		"persistent = 10",
		"user-agent = CustomAgent/1.0",
		"half-internet-routes = 1",
		"pppd-ipparam = ignored",
	}, "\n")

	cfg, warnings, err := ParseConfigText(input)
	if err != nil {
		t.Fatalf("ParseConfigText: %v", err)
	}
	if cfg.Host != "198.51.100.10" || cfg.Port != 10443 {
		t.Fatalf("host:port = %s:%d, want 198.51.100.10:10443", cfg.Host, cfg.Port)
	}
	if cfg.Username != "alice" || cfg.Password.Reveal() != "s3cr=t" || cfg.Realm != "corp" {
		t.Fatalf("credentials = %+v", cfg)
	}
	if cfg.DNS != DNSOff {
		t.Fatalf("DNS = %q, want off", cfg.DNS)
	}
	if !cfg.Reconnect.Enabled || cfg.Reconnect.Delay != 10*time.Second {
		t.Fatalf("Reconnect = %+v, want enabled 10s", cfg.Reconnect)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %+v, want 2 warnings", warnings)
	}

	formattedMasked := FormatConfigText(cfg, false)
	if strings.Contains(formattedMasked, "s3cr=t") {
		t.Fatalf("FormatConfigText(includePassword=false) leaked password:\n%s", formattedMasked)
	}
	if !strings.Contains(formattedMasked, SavedPasswordComment) {
		t.Fatalf("FormatConfigText(includePassword=false) missing saved password comment:\n%s", formattedMasked)
	}

	formattedFull := FormatConfigText(cfg, true)
	roundTrip, _, err := ParseConfigText(formattedFull)
	if err != nil {
		t.Fatalf("round-trip ParseConfigText: %v", err)
	}
	if roundTrip.Host != cfg.Host || roundTrip.Port != cfg.Port ||
		roundTrip.Username != cfg.Username || roundTrip.Password != cfg.Password ||
		roundTrip.Realm != cfg.Realm || roundTrip.DNS != cfg.DNS ||
		roundTrip.Reconnect != cfg.Reconnect {
		t.Fatalf("round-trip mismatch:\ngot:  %+v\nwant: %+v", roundTrip, cfg)
	}
}
