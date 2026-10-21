package fortivpn

import (
	"log/slog"
	"net/netip"
	"strings"
	"time"
)

const (
	DefaultPort           = 443
	DefaultUserAgent      = "Mozilla/5.0 SV1"
	DefaultReconnectDelay = 5 * time.Second
)

// Secret holds a sensitive credential (such as a password) that masks itself in
// fmt and slog output while serializing as a plain JSON string for IPC.
type Secret string

// NewSecret wraps a raw credential string into a Secret.
func NewSecret(v string) Secret     { return Secret(v) }
func (Secret) String() string       { return "******" }
func (Secret) GoString() string     { return `"******"` }
func (Secret) LogValue() slog.Value { return slog.StringValue("******") }
func (s Secret) Reveal() string     { return string(s) }
func (s Secret) IsEmpty() bool      { return len(s) == 0 }

// AuthMethod selects the authentication flow.
type AuthMethod string

const (
	AuthPassword AuthMethod = "password"
	AuthSAML     AuthMethod = "saml"
)

// RouteMode controls how tunnel routes are installed on the host.
type RouteMode string

const (
	RouteGateway RouteMode = "gateway"
	RouteFull    RouteMode = "full"
	RouteNone    RouteMode = "none"
)

// DNSMode controls how tunnel DNS servers and domains are applied on the host.
type DNSMode string

const (
	DNSSplit DNSMode = "split"
	DNSFull  DNSMode = "full"
	DNSOff   DNSMode = "off"
)

// ClientCert configures optional TLS client certificate authentication using PEM files.
type ClientCert struct {
	CertPath string `json:"certPath"`
	KeyPath  string `json:"keyPath"`
}

// Reconnect configures automatic reconnection on transient link failures.
type Reconnect struct {
	Enabled     bool          `json:"enabled"`
	Delay       time.Duration `json:"delay,omitempty"`
	MaxAttempts int           `json:"maxAttempts,omitempty"`
}

// Config is the complete configuration for a Fortinet SSL-VPN session.
type Config struct {
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	Username     string         `json:"username"`
	Password     Secret         `json:"password,omitempty"`
	Realm        string         `json:"realm,omitempty"`
	Auth         AuthMethod     `json:"auth"`
	TrustedCerts []string       `json:"trustedCerts,omitempty"`
	ClientCert   *ClientCert    `json:"clientCert,omitempty"`
	UserAgent    string         `json:"userAgent,omitempty"`
	LegacyTLS    bool           `json:"legacyTls,omitempty"`
	Routes       RouteMode      `json:"routes"`
	ExtraRoutes  []netip.Prefix `json:"extraRoutes,omitempty"`
	DNS          DNSMode        `json:"dns"`
	DNSDomains   []string       `json:"dnsDomains,omitempty"`
	Reconnect    Reconnect      `json:"reconnect"`
}

// Normalized returns a copy of Config with whitespace trimmed and default values filled.
func (c Config) Normalized() Config {
	c.Host = strings.TrimSpace(c.Host)
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	c.Username = strings.TrimSpace(c.Username)
	c.Realm = strings.TrimSpace(c.Realm)
	if c.Auth == "" {
		c.Auth = AuthPassword
	}
	if c.Routes == "" {
		c.Routes = RouteGateway
	}
	if c.DNS == "" {
		c.DNS = DNSSplit
	}
	c.UserAgent = strings.TrimSpace(c.UserAgent)
	if c.Reconnect.Enabled && c.Reconnect.Delay <= 0 {
		c.Reconnect.Delay = DefaultReconnectDelay
	}

	if len(c.TrustedCerts) > 0 {
		seen := make(map[string]struct{}, len(c.TrustedCerts))
		certs := make([]string, 0, len(c.TrustedCerts))
		for _, raw := range c.TrustedCerts {
			d := strings.ToLower(strings.TrimSpace(raw))
			if d == "" {
				continue
			}
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			certs = append(certs, d)
		}
		c.TrustedCerts = certs
	}

	if c.ClientCert != nil {
		certPath := strings.TrimSpace(c.ClientCert.CertPath)
		keyPath := strings.TrimSpace(c.ClientCert.KeyPath)
		if certPath == "" && keyPath == "" {
			c.ClientCert = nil
		} else {
			c.ClientCert = &ClientCert{CertPath: certPath, KeyPath: keyPath}
		}
	}

	if len(c.ExtraRoutes) > 0 {
		seen := make(map[netip.Prefix]struct{}, len(c.ExtraRoutes))
		routes := make([]netip.Prefix, 0, len(c.ExtraRoutes))
		for _, p := range c.ExtraRoutes {
			if !p.IsValid() {
				routes = append(routes, p)
				continue
			}
			masked := p.Masked()
			if _, ok := seen[masked]; ok {
				continue
			}
			seen[masked] = struct{}{}
			routes = append(routes, masked)
		}
		c.ExtraRoutes = routes
	}

	if len(c.DNSDomains) > 0 {
		seen := make(map[string]struct{}, len(c.DNSDomains))
		domains := make([]string, 0, len(c.DNSDomains))
		for _, raw := range c.DNSDomains {
			d := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "."))
			if d == "" {
				continue
			}
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			domains = append(domains, d)
		}
		c.DNSDomains = domains
	}
	return c
}

// Validate checks whether the configuration is valid for establishing a connection.
// It is the single source of truth for configuration validation across CLI, GUI, and helper.
func (c Config) Validate() error {
	return c.validate(true)
}

// ValidateStructural validates all fields except requiring a non-empty password,
// allowing the desktop profile editor to validate profiles whose password is
// already stored in the vault.
func (c Config) ValidateStructural() error {
	return c.validate(false)
}

func (c Config) validate(requirePassword bool) error {
	n := c.Normalized()

	if n.Host == "" {
		return NewFieldError("host", "gateway host is required")
	}
	if strings.ContainsAny(n.Host, " \t\r\n/\\") {
		return NewFieldError("host", "gateway host must not contain whitespace or slashes")
	}
	if n.Port < 1 || n.Port > 65535 {
		return NewFieldError("port", "port must be between 1 and 65535")
	}

	switch n.Auth {
	case AuthPassword, AuthSAML:
	default:
		return NewFieldError("auth", "auth must be password or saml")
	}

	if n.Auth == AuthPassword {
		if n.Username == "" {
			return NewFieldError("username", "username is required for password authentication")
		}
		if strings.ContainsAny(n.Username, "\r\n") {
			return NewFieldError("username", "username must not contain newlines")
		}
		if requirePassword && n.Password.IsEmpty() {
			return NewFieldError("password", "password is required")
		}
		if strings.ContainsAny(n.Password.Reveal(), "\r\n") {
			return NewFieldError("password", "password must not contain newlines")
		}
	}

	if strings.ContainsAny(n.Realm, " \t\r\n") {
		return NewFieldError("realm", "realm must not contain whitespace")
	}

	for _, digest := range n.TrustedCerts {
		if !IsValidCertDigest(digest) {
			return NewFieldError("trustedCerts", "trusted certificate digest must be 64 lower-case hex characters (SHA-256)")
		}
	}

	if n.ClientCert != nil {
		if n.ClientCert.CertPath == "" || n.ClientCert.KeyPath == "" {
			return NewFieldError("clientCert", "both client certificate and key paths are required")
		}
		if strings.ContainsAny(n.ClientCert.CertPath, "\r\n") || strings.ContainsAny(n.ClientCert.KeyPath, "\r\n") {
			return NewFieldError("clientCert", "client certificate paths must not contain newlines")
		}
	}

	if strings.ContainsAny(n.UserAgent, "\r\n") {
		return NewFieldError("userAgent", "user-agent must not contain newlines")
	}

	switch n.Routes {
	case RouteGateway, RouteFull, RouteNone:
	default:
		return NewFieldError("routes", "routes mode must be gateway, full, or none")
	}

	for _, pfx := range n.ExtraRoutes {
		if !pfx.IsValid() || !pfx.Addr().Is4() {
			return NewFieldError("extraRoutes", "extra route must be a valid IPv4 CIDR prefix")
		}
		if pfx.Bits() == 0 {
			return NewFieldError("extraRoutes", "extra route must not be 0.0.0.0/0; use routes=full instead")
		}
		addr := pfx.Addr()
		if addr.IsLoopback() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
			return NewFieldError("extraRoutes", "extra route must not target loopback, multicast, link-local, or unspecified addresses")
		}
	}

	switch n.DNS {
	case DNSSplit, DNSFull, DNSOff:
	default:
		return NewFieldError("dns", "dns mode must be split, full, or off")
	}

	for _, domain := range n.DNSDomains {
		if !isValidDomain(domain) {
			return NewFieldError("dnsDomains", "invalid DNS domain: "+domain)
		}
	}

	if n.Reconnect.MaxAttempts < 0 {
		return NewFieldError("reconnect", "reconnect maxAttempts must not be negative")
	}
	return nil
}

// IsValidCertDigest reports whether digest is a 64-character lower-case hexadecimal SHA-256 string.
func IsValidCertDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for i := 0; i < len(digest); i++ {
		c := digest[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func isValidDomain(d string) bool {
	if len(d) == 0 || len(d) > 253 {
		return false
	}
	labels := strings.Split(d, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}
