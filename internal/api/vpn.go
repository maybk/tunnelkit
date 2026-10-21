package api

import (
	"context"

	"tunnelkit/internal/vpn"
)

// VPNAPI 暴露 VPN 连接控制、交互式验证应答与日志查询接口。
type VPNAPI struct {
	service *vpn.Service
}

func NewVPNAPI(service *vpn.Service) *VPNAPI {
	return &VPNAPI{service: service}
}

func (a *VPNAPI) State() vpn.State {
	return a.service.State()
}

// Connect 发起连接。使用 WithoutCancel，避免前端调用上下文结束时中断授权或回滚流程；中止请调用 Cancel。
func (a *VPNAPI) Connect(ctx context.Context, profileID string) error {
	return a.service.Connect(context.WithoutCancel(ctx), profileID)
}

// Cancel 中止正在进行的连接。
func (a *VPNAPI) Cancel() {
	a.service.Cancel()
}

// Answer 提交 OTP / FTM Push / SAML 交互验证响应。
func (a *VPNAPI) Answer(ctx context.Context, promptID, value string, cancel bool) error {
	return a.service.Answer(context.WithoutCancel(ctx), promptID, value, cancel)
}

func (a *VPNAPI) Disconnect(ctx context.Context) error {
	return a.service.Disconnect(context.WithoutCancel(ctx))
}

func (a *VPNAPI) Log(profileID string) (vpn.LogView, error) {
	return a.service.Log(profileID)
}

func (a *VPNAPI) ClearLog(profileID string) error {
	return a.service.ClearLog(profileID)
}
