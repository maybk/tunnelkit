package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

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

// trayController 维护菜单栏图标与菜单；在 Wails 主线程事件循环就绪前不会调用 InvokeAsync，
// 启动阶段收到的 refresh 请求会被合并到 markReady 后执行。
type trayController struct {
	mu                sync.Mutex
	ready             bool
	pending           bool
	selectedProfileID string
	rebuild           func()
}

func (t *trayController) setSelectedProfileID(id string) {
	t.mu.Lock()
	t.selectedProfileID = id
	t.mu.Unlock()
}

func (t *trayController) syncSelectedProfileID(state vpn.State, profiles []profile.Profile) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.selectedProfileID = resolveSelectedProfileID(state, profiles, t.selectedProfileID)
	return t.selectedProfileID
}

func (t *trayController) refresh() {
	t.mu.Lock()
	if !t.ready || t.rebuild == nil {
		t.pending = true
		t.mu.Unlock()
		return
	}
	fn := t.rebuild
	t.mu.Unlock()
	go fn()
}

func (t *trayController) markReady() {
	t.mu.Lock()
	t.ready = true
	shouldRun := t.pending && t.rebuild != nil
	t.pending = false
	fn := t.rebuild
	t.mu.Unlock()
	if shouldRun {
		go fn()
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

	showWindow := func() { showMainWindow(mainWindow, icons.appIcon) }
	if runtime.GOOS != "darwin" {
		// Windows 与 Linux（StatusNotifierItem Activate）下左键点击托盘图标直接唤起主窗口，右键打开托盘菜单；
		// macOS 保持 clickHandler 为空以使用系统原生的左/右键弹出菜单栏行为。
		tray.OnClick(showWindow)
	}
	runAsync := func(name string, fn func(context.Context) error) {
		go func() {
			if err := fn(context.Background()); err != nil {
				slog.Warn("菜单栏操作失败", "action", name, "error", err)
				t.refresh()
			}
		}()
	}

	applyMenu := func(state vpn.State, profiles []profile.Profile, selectedID string) {
		menu := wailsApp.NewMenu()
		menu.Add("仪表板").OnClick(func(*application.Context) { showWindow() })
		menu.AddSeparator()

		nodeMenu := menu.AddSubmenu("节点")
		if len(profiles) == 0 {
			nodeMenu.Add("暂无节点").SetEnabled(false)
		} else {
			for _, p := range profiles {
				item := nodeMenu.AddRadio(p.Name, p.ID == selectedID)
				item.SetEnabled(state.Status != vpn.StatusDisconnecting)
				profileID := p.ID
				item.OnClick(func(*application.Context) {
					t.setSelectedProfileID(profileID)
					if state.Status == vpn.StatusDisconnected || state.ProfileID != profileID {
						runAsync("connect", func(ctx context.Context) error {
							return vpnService.Connect(ctx, profileID)
						})
					}
				})
			}
		}

		connectedChecked, connectEnabled := trayConnectState(state, selectedID)
		connectItem := menu.AddCheckbox("连接", connectedChecked)
		connectItem.SetEnabled(connectEnabled)
		connectItem.OnClick(func(*application.Context) {
			switch state.Status {
			case vpn.StatusConnected:
				runAsync("disconnect", vpnService.Disconnect)
			case vpn.StatusConnecting:
				vpnService.Cancel()
			case vpn.StatusDisconnected:
				if selectedID != "" {
					runAsync("connect", func(ctx context.Context) error {
						return vpnService.Connect(ctx, selectedID)
					})
				}
			}
		})

		menu.AddSeparator()
		quitItem := menu.Add("退出")
		quitItem.SetAccelerator("CmdOrCtrl+q")
		quitItem.OnClick(func(*application.Context) { wailsApp.Quit() })
		tray.SetMenu(menu)
	}

	loadSnapshot := func() (vpn.State, []profile.Profile, string) {
		state := vpnService.State()
		profiles, err := profileStore.List()
		if err != nil {
			slog.Warn("菜单栏读取连接配置失败", "error", err)
		}
		selectedID := t.syncSelectedProfileID(state, profiles)
		return state, profiles, selectedID
	}

	t.mu.Lock()
	t.rebuild = func() {
		state, profiles, selectedID := loadSnapshot()
		application.InvokeAsync(func() {
			applyMenu(state, profiles, selectedID)
		})
	}
	t.mu.Unlock()

	// 事件循环尚未启动，首次构建需同步执行；待平台事件循环启动后再开放异步派发。
	state, profiles, selectedID := loadSnapshot()
	applyMenu(state, profiles, selectedID)

	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		t.markReady()
	})
}

func resolveSelectedProfileID(state vpn.State, profiles []profile.Profile, currentID string) string {
	if hasProfile(profiles, state.ProfileID) {
		return state.ProfileID
	}
	if hasProfile(profiles, currentID) {
		return currentID
	}
	if len(profiles) > 0 {
		return profiles[0].ID
	}
	return ""
}

func hasProfile(profiles []profile.Profile, id string) bool {
	if id == "" {
		return false
	}
	for _, p := range profiles {
		if p.ID == id {
			return true
		}
	}
	return false
}

func trayConnectState(state vpn.State, selectedProfileID string) (checked bool, enabled bool) {
	checked = state.Status == vpn.StatusConnected || state.Status == vpn.StatusConnecting
	enabled = state.Status != vpn.StatusDisconnecting && (checked || selectedProfileID != "")
	return checked, enabled
}
