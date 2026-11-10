package sslvpn

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// RedactConfigXML masks sensitive attributes and element contents before export.
// Invalid documents are omitted rather than returned with partially redacted data.
func RedactConfigXML(data []byte) []byte {
	var out bytes.Buffer
	decoder := xml.NewDecoder(bytes.NewReader(data))
	encoder := xml.NewEncoder(&out)
	depth, roots := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return []byte("<redacted>Invalid XML omitted</redacted>")
		}
		switch item := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return []byte("<redacted/>")
				}
			}
			secret := sensitiveXMLName(item.Name.Local)
			for i := range item.Attr {
				if secret || sensitiveXMLName(item.Attr[i].Name.Local) {
					item.Attr[i].Value = "REDACTED"
				}
			}
			if err := encoder.EncodeToken(item); err != nil {
				return []byte("<redacted/>")
			}
			if secret {
				if err := decoder.Skip(); err != nil {
					return []byte("<redacted/>")
				}
				if err := encoder.EncodeToken(xml.CharData("REDACTED")); err != nil {
					return []byte("<redacted/>")
				}
				if err := encoder.EncodeToken(item.End()); err != nil {
					return []byte("<redacted/>")
				}
			} else {
				depth++
			}
		case xml.EndElement:
			depth--
			if err := encoder.EncodeToken(token); err != nil {
				return []byte("<redacted/>")
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(item)) != "" {
				return []byte("<redacted/>")
			}
			if err := encoder.EncodeToken(token); err != nil {
				return []byte("<redacted/>")
			}
		case xml.Comment, xml.Directive, xml.ProcInst:
			// Comments and processing instructions are not needed for diagnostics.
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return []byte("<redacted/>")
			}
		}
	}
	if roots != 1 || depth != 0 {
		return []byte("<redacted/>")
	}
	if err := encoder.Flush(); err != nil {
		return []byte("<redacted/>")
	}
	return out.Bytes()
}

func sensitiveXMLName(name string) bool {
	name = strings.ToLower(name)
	if name == "user" || name == "username" || name == "group" {
		return true
	}
	for _, key := range []string{"password", "passwd", "passphrase", "credential", "secret", "cookie", "token", "otp", "authorization", "api_key", "api-key"} {
		if strings.Contains(name, key) {
			return true
		}
	}
	return false
}
