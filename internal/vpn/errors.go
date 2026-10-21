package vpn

import (
	"strings"

	"tunnelkit/fortivpn"
)

// MapError 将内核返回的英文结构化错误码映射为用户可读的中文提示（唯一的文案映射出口）。
func MapError(fe *fortivpn.Error) *Error {
	if fe == nil {
		return nil
	}
	switch fe.Code {
	case fortivpn.CodeCertUntrusted:
		return &Error{
			Message:     "网关证书未受信任",
			Hint:        "网关使用了自签名或内网签发的证书。确认该网关属于你的组织后，可选择「信任此证书」并重新连接。",
			TrustedCert: strings.ToLower(strings.TrimSpace(fe.Fingerprint)),
		}
	case fortivpn.CodeAuthFailed:
		return &Error{
			Message: "用户名或密码错误",
			Hint:    "网关拒绝了本次认证，请在「配置」中检查账号、密码或 Realm。",
		}
	case fortivpn.CodeOTPRequired:
		return &Error{
			Message: "需要输入动态验证码",
			Hint:    "网关要求完成双因素验证，请重新连接并输入验证码。",
		}
	case fortivpn.CodeOTPInvalid:
		return &Error{
			Message: "验证码错误",
			Hint:    "网关拒绝了输入的动态验证码，请重新连接并输入最新验证码。",
		}
	case fortivpn.CodePromptTimeout:
		return &Error{
			Message: "等待验证超时",
			Hint:    "未在规定时间内完成双因素或单点登录验证，请重新发起连接。",
		}
	case fortivpn.CodeGatewayUnreachable:
		return &Error{
			Message: "无法连接到网关",
			Hint:    "请检查网络连接、网关地址与端口是否正确。",
		}
	case fortivpn.CodeTunnelDenied:
		return &Error{
			Message: "网关拒绝建立隧道",
			Hint:    "当前账号可能未开通 SSL-VPN 隧道模式权限，或网关已禁用隧道模式。",
		}
	case fortivpn.CodePPPFailed:
		return &Error{
			Message: "隧道协议协商失败",
			Hint:    "PPP 链路协商未完成，请重试或查看连接日志。",
		}
	case fortivpn.CodePeerDead, fortivpn.CodeConnectionLost:
		return &Error{
			Message: "连接已意外断开",
			Hint:    "与网关的隧道连接中断，可重新连接或在配置中开启「自动重连」。",
		}
	case fortivpn.CodeDeviceFailed:
		return &Error{
			Message: "创建虚拟网卡失败",
			Hint:    "请确认已授予管理员权限且虚拟网卡组件完整，详情见连接日志。",
		}
	case fortivpn.CodeNetcfgFailed:
		return &Error{
			Message: "配置系统网络失败",
			Hint:    "路由或 DNS 变更已自动回滚，请在「日志」中查看详细原因。",
		}
	case fortivpn.CodeInvalidConfig:
		return &Error{
			Message: "配置参数无效",
			Hint:    fe.Detail,
		}
	case fortivpn.CodeUnsupportedPlatform:
		return &Error{
			Message: "当前平台暂不支持建立隧道",
			Hint:    fe.Detail,
		}
	default:
		return &Error{
			Message: "连接未能建立",
			Hint:    "请在「日志」中查看详细输出。",
		}
	}
}
