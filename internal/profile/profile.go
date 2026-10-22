// Package profile 负责 VPN 服务配置模型、校验、v1->v2 迁移、持久化与密码加密存储。
package profile

import (
	"net/netip"
	"strings"
	"time"

	"tunnelkit/fortivpn"
)

// Profile 表示一个已保存的 VPN 服务配置（不含密码明文）。
type Profile struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Realm       string   `json:"realm,omitempty"`
	TrustedCert string   `json:"trustedCert,omitempty"`
	Auth        string   `json:"auth"`                  // password | saml
	Routes      string   `json:"routes"`                // gateway | full | none
	ExtraRoutes []string `json:"extraRoutes,omitempty"` // IPv4 CIDR 列表
	DNS         string   `json:"dns"`                   // split | full | off
	DNSDomains  []string `json:"dnsDomains,omitempty"`
	Reconnect   bool     `json:"reconnect"`
	UserAgent   string   `json:"userAgent,omitempty"`
	LegacyTLS   bool     `json:"legacyTls,omitempty"`
	ClientCert  string   `json:"clientCert,omitempty"`
	ClientKey   string   `json:"clientKey,omitempty"`
	HasPassword bool     `json:"hasPassword"`
	UpdatedAt   string   `json:"updatedAt"`
}

// Input 是创建或更新服务配置的输入参数。Password 为空时表示保留已保存的密码。
type Input struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	Realm       string   `json:"realm,omitempty"`
	TrustedCert string   `json:"trustedCert,omitempty"`
	Auth        string   `json:"auth,omitempty"`
	Routes      string   `json:"routes,omitempty"`
	ExtraRoutes []string `json:"extraRoutes,omitempty"`
	DNS         string   `json:"dns,omitempty"`
	DNSDomains  []string `json:"dnsDomains,omitempty"`
	Reconnect   bool     `json:"reconnect"`
	UserAgent   string   `json:"userAgent,omitempty"`
	LegacyTLS   bool     `json:"legacyTls,omitempty"`
	ClientCert  string   `json:"clientCert,omitempty"`
	ClientKey   string   `json:"clientKey,omitempty"`
}

// ToProfile 将输入参数转换为非敏感字段的 Profile 结构。
func (in Input) ToProfile() Profile {
	return Profile{
		ID:          in.ID,
		Name:        in.Name,
		Host:        in.Host,
		Port:        in.Port,
		Username:    in.Username,
		Realm:       in.Realm,
		TrustedCert: in.TrustedCert,
		Auth:        in.Auth,
		Routes:      in.Routes,
		ExtraRoutes: in.ExtraRoutes,
		DNS:         in.DNS,
		DNSDomains:  in.DNSDomains,
		Reconnect:   in.Reconnect,
		UserAgent:   in.UserAgent,
		LegacyTLS:   in.LegacyTLS,
		ClientCert:  in.ClientCert,
		ClientKey:   in.ClientKey,
	}
}

// ParseText 将 openfortivpn 兼容格式的配置文本解析为表单 Input。
func ParseText(base Input, text string) (Input, []fortivpn.Warning, error) {
	cfg, warnings, err := fortivpn.ParseConfigText(text)
	if err != nil {
		return Input{}, nil, err
	}
	return FromConfig(base, cfg), warnings, nil
}

// FormatText 将当前表单 Input 渲染为 openfortivpn 兼容格式的配置文本（不落盘）。
func FormatText(in Input, hasSavedPassword bool) string {
	if !hasSavedPassword &&
		in.Host == "" &&
		(in.Port == 0 || in.Port == DefaultPort) &&
		in.Username == "" &&
		in.Password == "" &&
		in.Realm == "" &&
		in.TrustedCert == "" &&
		len(in.ExtraRoutes) == 0 &&
		len(in.DNSDomains) == 0 {
		return ""
	}
	pwd := in.Password
	includePassword := true
	if pwd == "" && hasSavedPassword {
		pwd = "__SAVED__"
		includePassword = false
	}
	cfg := in.ToProfile().ToConfig(pwd)
	return fortivpn.FormatConfigText(cfg, includePassword)
}

// ToConfig 将桌面端 Profile 转换为内核 fortivpn.Config（唯一的转换入口）。
func (p Profile) ToConfig(password string) fortivpn.Config {
	var trustedCerts []string
	if strings.TrimSpace(p.TrustedCert) != "" {
		trustedCerts = []string{strings.ToLower(strings.TrimSpace(p.TrustedCert))}
	}

	var extraRoutes []netip.Prefix
	for _, raw := range p.ExtraRoutes {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		pfx, err := netip.ParsePrefix(trimmed)
		if err != nil {
			// 保留无效 Prefix 以便 fortivpn.Config.Validate() 统一报错。
			extraRoutes = append(extraRoutes, netip.Prefix{})
			continue
		}
		extraRoutes = append(extraRoutes, pfx)
	}

	var clientCert *fortivpn.ClientCert
	if strings.TrimSpace(p.ClientCert) != "" || strings.TrimSpace(p.ClientKey) != "" {
		clientCert = &fortivpn.ClientCert{
			CertPath: strings.TrimSpace(p.ClientCert),
			KeyPath:  strings.TrimSpace(p.ClientKey),
		}
	}

	auth := fortivpn.AuthMethod(strings.TrimSpace(p.Auth))
	if auth == "" {
		auth = fortivpn.AuthPassword
	}
	routes := fortivpn.RouteMode(strings.TrimSpace(p.Routes))
	if routes == "" {
		routes = fortivpn.RouteGateway
	}
	dns := fortivpn.DNSMode(strings.TrimSpace(p.DNS))
	if dns == "" {
		dns = fortivpn.DNSSplit
	}

	return fortivpn.Config{
		Host:         strings.TrimSpace(p.Host),
		Port:         p.Port,
		Username:     strings.TrimSpace(p.Username),
		Password:     fortivpn.NewSecret(password),
		Realm:        strings.TrimSpace(p.Realm),
		Auth:         auth,
		TrustedCerts: trustedCerts,
		ClientCert:   clientCert,
		UserAgent:    strings.TrimSpace(p.UserAgent),
		LegacyTLS:    p.LegacyTLS,
		Routes:       routes,
		ExtraRoutes:  extraRoutes,
		DNS:          dns,
		DNSDomains:   append([]string(nil), p.DNSDomains...),
		Reconnect: fortivpn.Reconnect{
			Enabled: p.Reconnect,
			Delay:   5 * time.Second,
		},
	}
}

// FromConfig 将内核 fortivpn.Config 映射回 Input（供文本模式解析使用）。
func FromConfig(base Input, cfg fortivpn.Config) Input {
	cfg = cfg.Normalized()
	out := base
	out.Host = cfg.Host
	out.Port = cfg.Port
	out.Username = cfg.Username
	if !cfg.Password.IsEmpty() {
		out.Password = cfg.Password.Reveal()
	}
	out.Realm = cfg.Realm
	if len(cfg.TrustedCerts) > 0 {
		out.TrustedCert = cfg.TrustedCerts[0]
	} else {
		out.TrustedCert = ""
	}
	out.Auth = string(cfg.Auth)
	out.Routes = string(cfg.Routes)
	out.ExtraRoutes = make([]string, 0, len(cfg.ExtraRoutes))
	for _, r := range cfg.ExtraRoutes {
		if r.IsValid() {
			out.ExtraRoutes = append(out.ExtraRoutes, r.String())
		}
	}
	out.DNS = string(cfg.DNS)
	out.DNSDomains = append([]string(nil), cfg.DNSDomains...)
	out.Reconnect = cfg.Reconnect.Enabled
	out.UserAgent = cfg.UserAgent
	out.LegacyTLS = cfg.LegacyTLS
	if cfg.ClientCert != nil {
		out.ClientCert = cfg.ClientCert.CertPath
		out.ClientKey = cfg.ClientCert.KeyPath
	} else {
		out.ClientCert = ""
		out.ClientKey = ""
	}
	return out
}
