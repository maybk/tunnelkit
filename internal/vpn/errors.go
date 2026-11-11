package vpn

import (
	"errors"
	"strings"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
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
			Hint:    "系统网络配置或恢复未完成，恢复记录会保留以便重试；请在「日志」中查看详细原因。",
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

func errControlStreamLost(connecting bool) *Error {
	msg := "连接已意外断开"
	if connecting {
		msg = "连接未能建立"
	}
	return &Error{
		Message: msg,
		Hint:    "与后台辅助进程通信中断，恢复记录已保留；请重新连接，或在「日志」中查看详细输出。",
	}
}

func errSessionUnconfirmed() *Error {
	return &Error{
		Message: "无法确认已有隧道状态",
		Hint:    "暂无法连接后台辅助进程，恢复记录已保留；请稍后重新连接，或在退出应用时重试清理。",
	}
}

func errPriorSessionPending() *Error {
	return &Error{
		Message: "上一会话尚未完成清理",
		Hint:    "后台辅助进程仍在收尾，恢复记录已保留；请稍候再重新发起连接。",
	}
}

func errSessionRead() *Error {
	return &Error{
		Message: "读取会话恢复记录失败",
		Hint:    "本地会话恢复文件无法解析或解密，为保护当前路由与 DNS 未自动覆盖；请重启应用重试或查看日志。",
	}
}

func errDisconnectStop() *Error {
	return &Error{
		Message: "断开连接未完成",
		Hint:    "未能向后台辅助进程确认停止指令，恢复记录已保留；请点击「连接」重新接管，或在退出应用时重试清理。",
	}
}

func errDisconnectTimeout() *Error {
	return &Error{
		Message: "等待隧道断开超时",
		Hint:    "VPN 隧道尚未确认关闭，恢复记录已保留；请稍后重新连接，或在退出应用时重试清理。",
	}
}

func errHelperExitTimeout() *Error {
	return &Error{
		Message: "等待辅助进程退出超时",
		Hint:    "后台辅助进程尚未完成退出，恢复记录已保留；请稍后重新连接，或在退出应用时重试清理。",
	}
}

func errNetworkRecoveryTimeout() *Error {
	return &Error{
		Message: "恢复系统网络超时",
		Hint:    "路由与 DNS 恢复尚未完成，恢复记录已保留；请稍后重新连接，或在退出应用时重试清理。",
	}
}

func errNetworkRecoveryFailed(legacy bool, err error) *Error {
	msg := "恢复系统网络失败"
	if legacy {
		msg = "恢复旧版网络配置失败"
	}
	if errors.Is(err, helperclient.ErrElevationCancelled) {
		return &Error{
			Message: msg,
			Hint:    "已取消管理员授权，路由与 DNS 恢复记录已保留；请重新连接或退出应用并完成授权。",
		}
	}
	return &Error{
		Message: msg,
		Hint:    "路由与 DNS 尚未确认恢复，恢复记录已保留；请稍后重新连接，或在「日志」中查看详细原因。",
	}
}

func errShutdownCleanup() *Error {
	return &Error{
		Message: "退出前清理 VPN 会话未完成",
		Hint:    "隧道或路由与 DNS 尚未确认清理完毕，恢复记录已保留；请稍后重试退出，或查看连接日志。",
	}
}
