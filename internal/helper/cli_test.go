package helper

import (
	"strings"
	"testing"
)

func TestParseServeArgs(t *testing.T) {
	t.Run("with_leading_serve_and_uid_alias", func(t *testing.T) {
		opts, err := ParseServeArgs([]string{
			"serve",
			"--endpoint", "/tmp/helper.sock",
			"--uid", "501",
			"--token-sha256", strings.Repeat("ab", 32),
		}, "1.2.3")
		if err != nil {
			t.Fatalf("ParseServeArgs failed: %v", err)
		}
		if opts.Endpoint != "/tmp/helper.sock" || opts.OwnerID != "501" || opts.TokenSHA256 != strings.Repeat("ab", 32) {
			t.Fatalf("unexpected opts: %+v", opts)
		}
		if opts.Version != "1.2.3" {
			t.Fatalf("unexpected paths/version: %+v", opts)
		}
	})

	t.Run("without_leading_serve_and_sid_alias", func(t *testing.T) {
		opts, err := ParseServeArgs([]string{
			"--endpoint", `\\.\pipe\TunnelKit-123`,
			"--sid", "S-1-5-21-100",
			"--token-sha256", strings.Repeat("01", 32),
		}, "1.0.0")
		if err != nil {
			t.Fatalf("ParseServeArgs failed: %v", err)
		}
		if opts.OwnerID != "S-1-5-21-100" {
			t.Fatalf("OwnerID = %q, want S-1-5-21-100", opts.OwnerID)
		}
	})

	t.Run("missing_required_flags", func(t *testing.T) {
		if _, err := ParseServeArgs([]string{"serve", "--endpoint", "/tmp/a.sock"}, "1.0.0"); err == nil {
			t.Fatal("expected error when --owner and --token-sha256 are missing")
		}
	})
}
