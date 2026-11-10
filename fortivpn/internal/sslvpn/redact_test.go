package sslvpn_test

import (
	"strings"
	"testing"

	"tunnelkit/fortivpn/internal/sslvpn"
)

func TestXMLRedactionCoversAttributesElementsAndMalformedInput(t *testing.T) {
	for _, input := range []string{
		`<sslvpn-tunnel SVPNCOOKIE="audit-cookie"/>`,
		`<sslvpn-tunnel token='audit-token'/>`,
		`<sslvpn-tunnel><password>audit-password</password></sslvpn-tunnel>`,
		`<sslvpn-tunnel><token><value>audit-token</value></token></sslvpn-tunnel>`,
		`<sslvpn-tunnel><!-- audit-comment --><user>audit-user</user></sslvpn-tunnel>`,
		`<sslvpn-tunnel password="audit-malformed`,
		`audit-malformed-document`,
	} {
		if strings.Contains(string(sslvpn.RedactConfigXML([]byte(input))), "audit") {
			t.Error("synthetic credential survived XML redaction")
		}
	}
	input := `<sslvpn-tunnel><assigned-addr ipv4="192.0.2.10"/><dns ip="192.0.2.53" domain="corp.example.com"/></sslvpn-tunnel>`
	out := string(sslvpn.RedactConfigXML([]byte(input)))
	if !strings.Contains(out, "192.0.2.10") || !strings.Contains(out, "corp.example.com") {
		t.Error("non-sensitive diagnostic fields were removed")
	}
}
