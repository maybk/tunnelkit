package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tunnelkit/internal/update"
	"tunnelkit/internal/vpn"
)

func newUpdater(version string, vpnService *vpn.Service, getApp func() *application.App) *update.Checker {
	return update.NewChecker(
		version,
		update.WithSelfUpdateCapable(update.CanInstallCurrentProcess()),
		update.WithPreUpdateHook(func(ctx context.Context) error {
			// 异常态也可能保留辅助会话，安装前必须确认停止与网络恢复完成，并进入维护门禁。
			return vpnService.BeginUpdate(ctx)
		}),
		update.WithProgressListener(func(p update.Progress) {
			if wailsApp := getApp(); wailsApp != nil {
				wailsApp.Event.Emit(EventUpdateProgress, p)
			}
		}),
		update.WithUpdateFinishedHook(func(restarting bool) {
			vpnService.EndUpdate(restarting)
			if restarting {
				if wailsApp := getApp(); wailsApp != nil {
					go wailsApp.Quit()
				}
			}
		}),
	)
}
