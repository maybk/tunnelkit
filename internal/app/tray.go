package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/vpn"
)

type iconSet struct {
	appIcon          []byte
	trayTemplateIcon []byte
	trayColorIcon    []byte
}

func loadIcons(assets fs.FS) (iconSet, error) {
	appIcon, err := fs.ReadFile(assets, "assets/icons/app-icon.png")
	if err != nil {
		return iconSet{}, fmt.Errorf("读取应用图标失败: %w", err)
	}
	trayTemplate, err := fs.ReadFile(assets, "assets/icons/tray-icon.png")
	if err != nil {
		return iconSet{}, fmt.Errorf("读取菜单栏模板图标失败: %w", err)
	}
	trayColor, err := fs.ReadFile(assets, "assets/icons/tray-icon-color.png")
	if err != nil {
		return iconSet{}, fmt.Errorf("读取托盘彩色图标失败: %w", err)
	}
	return iconSet{
		appIcon:          appIcon,
		trayTemplateIcon: trayTemplate,
		trayColorIcon:    trayColor,
	}, nil
}

func (s iconSet) trayIconForPlatform(goos string) (icon []byte, isTemplate bool) {
	if goos == "darwin" {
		return s.trayTemplateIcon, true
	}
	return s.trayColorIcon, false
}

// trayController 维护菜单栏图标与菜单；refresh 可在 init 之前安全调用（此时为空操作）。
type trayController struct {
	rebuild func()
}

func (t *trayController) refresh() {
	if t.rebuild != nil {
		application.InvokeAsync(t.rebuild)
	}
}

func (t *trayController) init(
	wailsApp *application.App,
	mainWindow application.Window,
	vpnService *vpn.Service,
	profileStore *profile.Store,
	icons iconSet,
) {
	tray := wailsApp.SystemTray.New()
	if icon, isTemplate := icons.trayIconForPlatform(runtime.GOOS); isTemplate {
		tray.SetTemplateIcon(icon)
	} else {
		tray.SetIcon(icon)
	}
	tray.SetTooltip(ApplicationName)

	showWindow := func() { mainWindow.Show().Focus() }
	runAsync := func(name string, fn func(context.Context) error) {
		go func() {
			if err := fn(context.Background()); err != nil {
				slog.Warn("菜单栏操作失败", "action", name, "error", err)
			}
		}()
	}

	t.rebuild = func() {
		state := vpnService.State()
		profiles, err := profileStore.List()
		if err != nil {
			slog.Warn("菜单栏读取连接配置失败", "error", err)
		}

		menu := wailsApp.NewMenu()
		menu.Add(trayStatusLabel(state)).SetEnabled(false)

		switch state.Status {
		case vpn.StatusConnected:
			menu.Add("断开「" + state.ProfileName + "」").OnClick(func(*application.Context) {
				runAsync("disconnect", vpnService.Disconnect)
			})
		case vpn.StatusConnecting:
			menu.Add("取消连接").OnClick(func(*application.Context) { vpnService.Cancel() })
		}

		if len(profiles) > 0 {
			menu.AddSeparator()
			for _, p := range profiles {
				isCurrent := state.Status != vpn.StatusDisconnected && state.ProfileID == p.ID
				label := "连接「" + p.Name + "」"
				if state.Status != vpn.StatusDisconnected && !isCurrent {
					label = "切换至「" + p.Name + "」"
				}
				item := menu.Add(label)
				item.SetEnabled(state.Status != vpn.StatusDisconnecting && !isCurrent)
				profileID := p.ID
				item.OnClick(func(*application.Context) {
					runAsync("connect", func(ctx context.Context) error { return vpnService.Connect(ctx, profileID) })
				})
			}
		}

		menu.AddSeparator()
		menu.Add("打开 TunnelKit").OnClick(func(*application.Context) { showWindow() })
		menu.Add("退出").OnClick(func(*application.Context) { wailsApp.Quit() })
		tray.SetMenu(menu)
	}

	// 事件循环尚未启动，首次构建需同步执行。
	t.rebuild()
}

func trayStatusLabel(state vpn.State) string {
	switch state.Status {
	case vpn.StatusConnected:
		return "● 已连接 · " + state.ProfileName
	case vpn.StatusConnecting:
		if state.Phase == fortivpn.PhaseReconnecting {
			return "↻ 重新连接中 · " + state.ProfileName
		}
		return "正在连接 · " + state.ProfileName
	case vpn.StatusDisconnecting:
		return "正在断开 · " + state.ProfileName
	default:
		return "未连接"
	}
}
