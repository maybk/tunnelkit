package fortivpn

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// SavedPasswordComment is emitted in formatted configuration text when a profile
// has a stored password that is intentionally omitted from the text view.
const SavedPasswordComment = "# password = 已保存，留空不修改"

// Warning reports a non-fatal compatibility note encountered while parsing an
// openfortivpn-style configuration text.
type Warning struct {
	Line    int    `json:"line"`
	Key     string `json:"key"`
	Message string `json:"message"`
}

func (w Warning) String() string {
	return fmt.Sprintf("line %d (%s): %s", w.Line, w.Key, w.Message)
}

// ParseConfigText parses an openfortivpn-compatible key = value configuration
// text into a Config. Unknown or deprecated keys produce Warnings rather than
// hard errors so existing openfortivpn config files can be imported smoothly.
func ParseConfigText(text string) (Config, []Warning, error) {
	cfg := Config{
		Port:   DefaultPort,
		Auth:   AuthPassword,
		Routes: RouteGateway,
		DNS:    DNSSplit,
	}

	var warnings []Warning
	explicitPortSeen := false
	dnsOff := false
	dnsModeExplicit := false

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for idx, rawLine := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		eqIdx := strings.IndexByte(rawLine, '=')
		if eqIdx < 0 {
			return Config{}, warnings, NewFieldError("configText", fmt.Sprintf("line %d: expected key = value", lineNum))
		}

		key := strings.ToLower(strings.TrimSpace(rawLine[:eqIdx]))
		val := strings.TrimSpace(rawLine[eqIdx+1:])
		if key == "" {
			return Config{}, warnings, NewFieldError("configText", fmt.Sprintf("line %d: missing option key", lineNum))
		}

		switch key {
		case "host":
			if host, portStr, ok := splitHostPortValue(val); ok {
				cfg.Host = host
				if !explicitPortSeen {
					p, err := strconv.Atoi(portStr)
					if err != nil {
						return Config{}, warnings, NewFieldError("port", fmt.Sprintf("line %d: invalid port %q", lineNum, portStr))
					}
					cfg.Port = p
				}
			} else {
				cfg.Host = val
			}
		case "port":
			explicitPortSeen = true
			p, err := strconv.Atoi(val)
			if err != nil {
				return Config{}, warnings, NewFieldError("port", fmt.Sprintf("line %d: invalid port %q", lineNum, val))
			}
			cfg.Port = p
		case "username":
			cfg.Username = val
		case "password":
			cfg.Password = Secret(val)
		case "realm":
			cfg.Realm = val
		case "trusted-cert":
			if val != "" {
				cfg.TrustedCerts = append(cfg.TrustedCerts, strings.ToLower(val))
			}
		case "saml-login":
			if isTruthy(val) {
				cfg.Auth = AuthSAML
			} else {
				cfg.Auth = AuthPassword
			}
		case "auth":
			cfg.Auth = AuthMethod(strings.ToLower(val))
		case "set-routes":
			if val == "0" || strings.EqualFold(val, "false") {
				cfg.Routes = RouteNone
			} else if val == "1" || strings.EqualFold(val, "true") {
				if cfg.Routes == RouteNone {
					cfg.Routes = RouteGateway
				}
			} else {
				return Config{}, warnings, NewFieldError("routes", fmt.Sprintf("line %d: invalid set-routes value %q", lineNum, val))
			}
		case "routes":
			cfg.Routes = RouteMode(strings.ToLower(val))
		case "extra-routes":
			for _, part := range splitList(val) {
				pfx, err := netip.ParsePrefix(part)
				if err != nil {
					return Config{}, warnings, NewFieldError("extraRoutes", fmt.Sprintf("line %d: invalid CIDR %q", lineNum, part))
				}
				cfg.ExtraRoutes = append(cfg.ExtraRoutes, pfx)
			}
		case "half-internet-routes":
			warnings = append(warnings, Warning{
				Line:    lineNum,
				Key:     key,
				Message: "half-internet-routes is automatic when routes = full",
			})
		case "set-dns", "pppd-use-peerdns":
			if val == "0" || strings.EqualFold(val, "false") {
				dnsOff = true
			}
		case "dns":
			dnsModeExplicit = true
			cfg.DNS = DNSMode(strings.ToLower(val))
		case "dns-domains":
			cfg.DNSDomains = append(cfg.DNSDomains, splitList(val)...)
		case "persistent":
			sec, err := strconv.Atoi(val)
			if err != nil || sec < 0 {
				return Config{}, warnings, NewFieldError("reconnect", fmt.Sprintf("line %d: invalid persistent value %q", lineNum, val))
			}
			if sec > 0 {
				cfg.Reconnect.Enabled = true
				cfg.Reconnect.Delay = time.Duration(sec) * time.Second
			} else {
				cfg.Reconnect.Enabled = false
			}
		case "user-agent":
			cfg.UserAgent = val
		case "min-tls", "seclevel-1", "cipher-list":
			cfg.LegacyTLS = true
			warnings = append(warnings, Warning{
				Line:    lineNum,
				Key:     key,
				Message: fmt.Sprintf("%s mapped to legacy-tls = 1", key),
			})
		case "legacy-tls":
			cfg.LegacyTLS = isTruthy(val)
		case "user-cert":
			if cfg.ClientCert == nil {
				cfg.ClientCert = &ClientCert{}
			}
			cfg.ClientCert.CertPath = val
		case "user-key":
			if cfg.ClientCert == nil {
				cfg.ClientCert = &ClientCert{}
			}
			cfg.ClientCert.KeyPath = val
		default:
			warnings = append(warnings, Warning{
				Line:    lineNum,
				Key:     key,
				Message: fmt.Sprintf("ignored unsupported option %q", key),
			})
		}
	}

	if dnsOff && !dnsModeExplicit {
		cfg.DNS = DNSOff
	}
	return cfg.Normalized(), warnings, nil
}

// FormatConfigText renders a Config in openfortivpn-compatible key = value format.
// If includePassword is false and a password is set (or hasSavedPasswordComment is used by callers),
// the password line is replaced by SavedPasswordComment.
func FormatConfigText(c Config, includePassword bool) string {
	n := c.Normalized()
	var lines []string

	if n.Host != "" {
		lines = append(lines, "host = "+n.Host)
	}
	lines = append(lines, fmt.Sprintf("port = %d", n.Port))
	if n.Username != "" {
		lines = append(lines, "username = "+n.Username)
	}
	if !n.Password.IsEmpty() {
		if includePassword {
			lines = append(lines, "password = "+n.Password.Reveal())
		} else {
			lines = append(lines, SavedPasswordComment)
		}
	}
	if n.Realm != "" {
		lines = append(lines, "realm = "+n.Realm)
	}
	if n.Auth == AuthSAML {
		lines = append(lines, "saml-login = 1")
	}
	for _, cert := range n.TrustedCerts {
		lines = append(lines, "trusted-cert = "+cert)
	}
	if n.Routes != "" && n.Routes != RouteGateway {
		if n.Routes == RouteNone {
			lines = append(lines, "set-routes = 0")
		} else {
			lines = append(lines, "routes = "+string(n.Routes))
		}
	}
	if len(n.ExtraRoutes) > 0 {
		parts := make([]string, 0, len(n.ExtraRoutes))
		for _, pfx := range n.ExtraRoutes {
			parts = append(parts, pfx.String())
		}
		lines = append(lines, "extra-routes = "+strings.Join(parts, ","))
	}
	if n.DNS != "" && n.DNS != DNSSplit {
		if n.DNS == DNSOff {
			lines = append(lines, "set-dns = 0")
		} else {
			lines = append(lines, "dns = "+string(n.DNS))
		}
	}
	if len(n.DNSDomains) > 0 {
		lines = append(lines, "dns-domains = "+strings.Join(n.DNSDomains, ","))
	}
	if n.Reconnect.Enabled {
		sec := int(n.Reconnect.Delay / time.Second)
		if sec <= 0 {
			sec = int(DefaultReconnectDelay / time.Second)
		}
		lines = append(lines, fmt.Sprintf("persistent = %d", sec))
	}
	if n.UserAgent != "" {
		lines = append(lines, "user-agent = "+n.UserAgent)
	}
	if n.LegacyTLS {
		lines = append(lines, "legacy-tls = 1")
	}
	if n.ClientCert != nil {
		if n.ClientCert.CertPath != "" {
			lines = append(lines, "user-cert = "+n.ClientCert.CertPath)
		}
		if n.ClientCert.KeyPath != "" {
			lines = append(lines, "user-key = "+n.ClientCert.KeyPath)
		}
	}
	return strings.Join(lines, "\n")
}

func splitHostPortValue(val string) (host, port string, ok bool) {
	colon := strings.LastIndexByte(val, ':')
	if colon <= 0 || colon == len(val)-1 {
		return "", "", false
	}
	if strings.IndexByte(val[:colon], ':') >= 0 {
		return "", "", false
	}
	p := val[colon+1:]
	for i := 0; i < len(p); i++ {
		if p[i] < '0' || p[i] > '9' {
			return "", "", false
		}
	}
	return val[:colon], p, true
}

func isTruthy(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitList(val string) []string {
	var out []string
	for _, item := range strings.FieldsFunc(val, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
