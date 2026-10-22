package profile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"tunnelkit/fortivpn"
)

const DefaultPort = fortivpn.DefaultPort

// NormalizeAndValidate 规范化并校验输入参数，核心协议字段复用 fortivpn.Config.Validate。
func NormalizeAndValidate(in Input, hasExistingPassword bool) (Input, error) {
	in.ID = strings.TrimSpace(in.ID)
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	in.Realm = strings.TrimSpace(in.Realm)
	in.TrustedCert = strings.ToLower(strings.TrimSpace(in.TrustedCert))
	in.Auth = strings.ToLower(strings.TrimSpace(in.Auth))
	in.Routes = strings.ToLower(strings.TrimSpace(in.Routes))
	in.DNS = strings.ToLower(strings.TrimSpace(in.DNS))
	in.UserAgent = strings.TrimSpace(in.UserAgent)
	in.ClientCert = strings.TrimSpace(in.ClientCert)
	in.ClientKey = strings.TrimSpace(in.ClientKey)
	in.ExtraRoutes = cleanStringList(in.ExtraRoutes)
	in.DNSDomains = cleanDomainList(in.DNSDomains)

	if in.Port == 0 {
		in.Port = DefaultPort
	}
	if in.Auth == "" {
		in.Auth = string(fortivpn.AuthPassword)
	}
	if in.Routes == "" {
		in.Routes = string(fortivpn.RouteGateway)
	}
	if in.DNS == "" {
		in.DNS = string(fortivpn.DNSSplit)
	}

	if in.ID != "" && !IsValidID(in.ID) {
		return Input{}, errors.New("配置 ID 格式无效")
	}
	if in.Name == "" {
		return Input{}, errors.New("请填写连接名称")
	}

	cfg := in.ToProfile().ToConfig(in.Password)
	var err error
	if hasExistingPassword && in.Password == "" {
		err = cfg.ValidateStructural()
	} else {
		err = cfg.Validate()
	}
	if err != nil {
		return Input{}, translateValidationError(err)
	}

	// 将 Normalized() 后的规范化结果同步回 Input。
	normCfg := cfg.Normalized()
	in.ExtraRoutes = make([]string, 0, len(normCfg.ExtraRoutes))
	for _, pfx := range normCfg.ExtraRoutes {
		if pfx.IsValid() {
			in.ExtraRoutes = append(in.ExtraRoutes, pfx.String())
		}
	}
	in.DNSDomains = normCfg.DNSDomains
	return in, nil
}

func translateValidationError(err error) error {
	fe, ok := fortivpn.AsError(err)
	if !ok {
		return err
	}
	switch fe.Field {
	case "host":
		if strings.Contains(fe.Detail, "required") {
			return errors.New("请填写网关地址")
		}
		return errors.New("网关地址不能包含空白字符或斜杠")
	case "port":
		return errors.New("端口必须在 1 到 65535 之间")
	case "auth":
		return errors.New("认证方式无效，仅支持密码或 SAML")
	case "username":
		if strings.Contains(fe.Detail, "required") {
			return errors.New("请填写用户名")
		}
		return errors.New("用户名格式无效")
	case "password":
		if strings.Contains(fe.Detail, "required") {
			return errors.New("请填写密码")
		}
		return errors.New("密码不能包含换行符")
	case "realm":
		return errors.New("Realm 不能包含空白字符")
	case "trustedCerts":
		return errors.New("受信任证书指纹必须是 64 位十六进制 SHA-256")
	case "clientCert":
		return errors.New("启用客户端证书时，必须同时填写证书与私钥路径")
	case "userAgent":
		return errors.New("User-Agent 格式无效")
	case "routes":
		return errors.New("路由模式无效")
	case "extraRoutes":
		if strings.Contains(fe.Detail, "0.0.0.0/0") {
			return errors.New("补充路由不支持 0.0.0.0/0，请将路由模式设为「全局路由」")
		}
		return errors.New("补充路由必须是合法的 IPv4 网段（如 10.10.0.0/16），且不能包含回环或链路本地地址")
	case "dns":
		return errors.New("DNS 模式无效")
	case "dnsDomains":
		return errors.New("内网域名格式无效")
	default:
		return errors.New("配置参数校验未通过: " + fe.Detail)
	}
}

// IsValidCertDigest 判断是否为 64 位小写十六进制 SHA-256 字符串。
func IsValidCertDigest(digest string) bool {
	return fortivpn.IsValidCertDigest(digest)
}

// IsValidID 校验配置 ID 是否为 32 位小写十六进制字符串（用于拼文件路径，防止路径穿越）。
func IsValidID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func cleanStringList(items []string) []string {
	var out []string
	for _, raw := range items {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		}) {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func cleanDomainList(items []string) []string {
	var out []string
	for _, raw := range cleanStringList(items) {
		trimmed := strings.ToLower(strings.Trim(raw, "."))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
