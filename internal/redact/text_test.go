package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsCredentialFormatsAndIsIdempotent(t *testing.T) {
	for _, input := range []string{
		`password="audit secret" host=vpn.example.com`,
		`{"credential":"audit secret","host":"vpn.example.com"}`,
		`password="audit \"quoted\" secret" token=audit-token`,
		"password=\"audit first\naudit second\"\nhost=vpn.example.com",
		"{\"password\":\n\"audit-multiline-json\"}",
		`{"token":["audit-one","audit-two"]}`,
		`{"secret":{"nested":"audit-value"}}`,
		`SVPNCOOKIE=audit-cookie; Path=/`,
		`Authorization: Bearer audit-token`,
		`Cookie: first=audit-one; second=audit-two`,
		`password="audit unterminated`,
		`proxy=https://audit-user:audit-pass@vpn.example.com:443/remote/login`,
		`header value Bearer audit.jwt%3Dtoken failed`,
		`header value Basic YXVkaXQ6c2VjcmV0 rejected`,
	} {
		output := Text(input)
		if strings.Contains(output, "audit") || strings.Contains(output, "YXVkaXQ") {
			t.Errorf("synthetic credential survived redaction: in=%q out=%q", input, output)
		}
		if Text(output) != output {
			t.Errorf("redaction was not idempotent: out=%q second=%q", output, Text(output))
		}
	}
	if got := Text(`password="audit secret" host=vpn.example.com`); !strings.Contains(got, "host=vpn.example.com") {
		t.Error("diagnostic fields were removed")
	}
	if got := Text(`Basic auth failed on vpn.example.com`); got != `Basic auth failed on vpn.example.com` {
		t.Errorf("ordinary English phrase was over-redacted: %q", got)
	}
	if got := Text(`https://alice:secret@vpn.example.com:443/remote`); got != `https://[REDACTED]@vpn.example.com:443/remote` {
		t.Errorf("unexpected URI redaction: %q", got)
	}
}
