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
	} {
		output := Text(input)
		if strings.Contains(output, "audit") {
			t.Error("synthetic credential survived redaction")
		}
		if Text(output) != output {
			t.Error("redaction was not idempotent")
		}
	}
	if got := Text(`password="audit secret" host=vpn.example.com`); !strings.Contains(got, "host=vpn.example.com") {
		t.Error("diagnostic fields were removed")
	}
}
