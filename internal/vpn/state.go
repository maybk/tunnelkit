// Package vpn 负责桌面端 VPN 连接状态机、helper IPC 会话恢复与事件流编排。
package vpn

import "tunnelkit/fortivpn"

// Status 表示 VPN 连接生命周期顶层状态（菜单栏与主界面共用）。
type Status string

const (
	StatusDisconnected  Status = "disconnected"
	StatusConnecting    Status = "connecting"
	StatusConnected     Status = "connected"
	StatusDisconnecting Status = "disconnecting"
)

// Error 表示最近一次连接或运行失败的用户可读原因。
type Error struct {
	Message     string `json:"message"`
	Hint        string `json:"hint,omitempty"`
	TrustedCert string `json:"trustedCert,omitempty"`
}

// State 是前端与菜单栏共用的唯一连接状态视图。
type State struct {
	Status      Status               `json:"status"`
	Phase       fortivpn.Phase       `json:"phase,omitempty"`
	ProfileID   string               `json:"profileId,omitempty"`
	ProfileName string               `json:"profileName,omitempty"`
	ConnectedAt string               `json:"connectedAt,omitempty"`
	Info        *fortivpn.TunnelInfo `json:"info,omitempty"`
	Stats       *fortivpn.Stats      `json:"stats,omitempty"`
	Prompt      *fortivpn.Prompt     `json:"prompt,omitempty"`
	Error       *Error               `json:"error,omitempty"`
}

// Active 判断指定配置是否正处于连接、已连接或断开中。
func (s State) Active(profileID string) bool {
	return s.ProfileID == profileID && s.Status != StatusDisconnected
}

// LogView 是单个连接配置的脱敏日志视图；UpdatedAt 为日志文件最后修改时间。
type LogView struct {
	ProfileID string `json:"profileId"`
	Content   string `json:"content"`
	Exists    bool   `json:"exists"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func statusForPhase(phase fortivpn.Phase) Status {
	switch phase {
	case fortivpn.PhaseConnected:
		return StatusConnected
	case fortivpn.PhaseDisconnecting:
		return StatusDisconnecting
	case fortivpn.PhaseDisconnected, "":
		return StatusDisconnected
	default:
		return StatusConnecting
	}
}
