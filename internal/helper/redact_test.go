package helper

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLogRedactionCoversQuotedValuesAndHeaders(t *testing.T) {
	for _, input := range []string{
		`password="audit secret" host=vpn.example.com`,
		`{"credential": "audit secret"}`,
		`otp: 'audit secret'`,
		`Authorization: Bearer audit-secret`,
		`Cookie: first=audit-secret; second=audit-secret`,
		`Set-Cookie: SVPNCOOKIE=audit-secret; Path=/`,
	} {
		if strings.Contains(RedactLogLine(input), "audit") {
			t.Error("synthetic credential survived log redaction")
		}
	}
}

func TestLoggerRedactsSensitiveGroupedAndBoundAttributes(t *testing.T) {
	w := &testBufferWriter{}
	logger := NewLogger(w).With("passphrase", "audit first secret").WithGroup("auth")
	logger.LogAttrs(context.Background(), slog.LevelInfo, "Authentication failed", slog.Group("details", slog.String("api_key", "audit second secret"), slog.String("host", "vpn.example.com")))
	if strings.Contains(w.String(), "audit") {
		t.Error("synthetic structured credential survived redaction")
	}
	if !strings.Contains(w.String(), "vpn.example.com") {
		t.Error("non-sensitive diagnostics were removed")
	}
}

func TestLoggerRedactsValuesInsideSensitiveGroup(t *testing.T) {
	w := &testBufferWriter{}
	NewLogger(w).With("host", "vpn.example.com").WithGroup("authorization").With("value", "audit-bound").Info("failure", "value", "audit-live")
	if strings.Contains(w.String(), "audit") {
		t.Error("sensitive group exposed its values")
	}
	if !strings.Contains(w.String(), "vpn.example.com") {
		t.Error("outer diagnostic attribute was removed")
	}
}
