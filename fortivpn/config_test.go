package fortivpn

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"testing"
)

func TestSecretMaskingAndJSON(t *testing.T) {
	s := Secret("my-super-secret-pw")
	if got := fmt.Sprintf("%s %v %#v", s, s, s); strings.Contains(got, "my-super-secret-pw") {
		t.Fatalf("fmt leaked secret: %s", got)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("connecting", "password", s)
	if strings.Contains(buf.String(), "my-super-secret-pw") {
		t.Fatalf("slog leaked secret: %s", buf.String())
	}

	raw, err := json.Marshal(struct {
		Password Secret `json:"password"`
	}{Password: s})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if string(raw) != `{"password":"my-super-secret-pw"}` {
		t.Fatalf("json.Marshal = %s, want plain JSON string for IPC", raw)
	}
}

func TestConfigValidate(t *testing.T) {
	validDigest := strings.Repeat("a", 64)
	base := Config{
		Host:         "vpn.example.com",
		Port:         10443,
		Username:     "alice",
		Password:     "secret",
		TrustedCerts: []string{strings.ToUpper(validDigest), validDigest},
		ExtraRoutes:  []netip.Prefix{netip.MustParsePrefix("172.16.10.2/32")},
		DNSDomains:   []string{".Corp.Example.com."},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
	norm := base.Normalized()
	if len(norm.TrustedCerts) != 1 || norm.TrustedCerts[0] != validDigest {
		t.Fatalf("TrustedCerts normalization failed: %v", norm.TrustedCerts)
	}
	if len(norm.DNSDomains) != 1 || norm.DNSDomains[0] != "corp.example.com" {
		t.Fatalf("DNSDomains normalization failed: %v", norm.DNSDomains)
	}

	cases := []struct {
		name      string
		mutate    func(*Config)
		wantField string
	}{
		{"empty host", func(c *Config) { c.Host = "  " }, "host"},
		{"host with space", func(c *Config) { c.Host = "vpn example.com" }, "host"},
		{"bad port", func(c *Config) { c.Port = 70000 }, "port"},
		{"empty username", func(c *Config) { c.Username = "" }, "username"},
		{"empty password", func(c *Config) { c.Password = "" }, "password"},
		{"bad cert digest", func(c *Config) { c.TrustedCerts = []string{"1234"} }, "trustedCerts"},
		{"default route in extraRoutes", func(c *Config) { c.ExtraRoutes = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")} }, "extraRoutes"},
		{"loopback in extraRoutes", func(c *Config) { c.ExtraRoutes = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")} }, "extraRoutes"},
		{"bad domain", func(c *Config) { c.DNSDomains = []string{"-bad.domain"} }, "dnsDomains"},
		{"incomplete clientCert", func(c *Config) { c.ClientCert = &ClientCert{CertPath: "/tmp/a.pem"} }, "clientCert"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			fe, ok := AsError(err)
			if !ok || fe.Code != CodeInvalidConfig || fe.Field != tc.wantField {
				t.Fatalf("Validate() = %+v, want CodeInvalidConfig with field %q", err, tc.wantField)
			}
		})
	}

	// ValidateStructural allows empty password when password is already in the vault.
	noPass := base
	noPass.Password = ""
	if err := noPass.ValidateStructural(); err != nil {
		t.Fatalf("ValidateStructural should allow empty password: %v", err)
	}
}

func TestErrorRetryable(t *testing.T) {
	if !NewError(CodePeerDead, "echo timeout").Retryable() {
		t.Fatal("CodePeerDead should be retryable")
	}
	if NewError(CodeAuthFailed, "bad credentials").Retryable() {
		t.Fatal("CodeAuthFailed should not be retryable")
	}
}
