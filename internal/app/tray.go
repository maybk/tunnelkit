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

// iconSet 汇总内嵌至二进制的应用与托盘运行时图标。
type iconSet struct {
	appIcon          []byte
	trayTemplateIcon []byte // macOS 菜单栏模板图像（单色 + Alpha）
	trayWindowsIcon  []byte // Windows 彩色多帧 ICO，按 SM_CXSMICON 选帧
	trayColorIcon    []byte // Linux StatusNotifierItem 彩色 PNG
}

// trayIcon 描述某个平台的托盘图标装配方式。
type trayIcon struct {
	icon     []byte
	template bool // 仅 macOS：交由 AppKit 按菜单栏外观着色
}

func loadIcons(assets fs.FS) (iconSet, error) {
	var icons iconSet
	files := []struct {
		path  string
		label string
		dst   *[]byte
	}{
		{"assets/icons/app-icon.png", "应用图标", &icons.appIcon},
		{"assets/icons/tray-icon.png", "菜单栏模板图标", &icons.trayTemplateIcon},
		{"assets/icons/tray-icon-windows.ico", "Windows 托盘图标", &icons.trayWindowsIcon},
		{"assets/icons/tray-icon-color.png", "托盘彩色图标", &icons.trayColorIcon},
	}
	for _, f := range files {
		data, err := fs.ReadFile(assets, f.path)
		if err != nil {
			return iconSet{}, fmt.Errorf("读取%s失败: %w", f.label, err)
		}
		*f.dst = data
	}
	return icons, nil
}

func (s iconSet) trayIconForPlatform(goos string) trayIcon {
	switch goos {
	case "darwin":
		return trayIcon{icon: s.trayTemplateIcon, template: true}
	case "windows":
		return trayIcon{icon: s.trayWindowsIcon}
	default:
		return trayIcon{icon: s.trayColorIcon}
	}
}

// trayController 维护菜单栏图标与菜单；在 Wails 主线程事件循环就绪前不会调用 InvokeAsync，
// 启动阶段与运行期并发收到的 refresh 请求都会串行合并执行，避免旧快照覆盖新状态。
type trayController struct {
	mu                sync.Mutex
	ready             bool
	pending           bool
	running           bool
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
	if !t.ready || t.rebuild == nil || t.running {
		t.pending = true
		t.mu.Unlock()
		return
	}
	t.pending = false
	t.running = true
	t.mu.Unlock()
	go t.runLoop()
}

func (t *trayController) markReady() {
	t.mu.Lock()
	t.ready = true
	if !t.pending || t.rebuild == nil || t.running {
		t.mu.Unlock()
		return
	}
	t.pending = false
	t.running = true
	t.mu.Unlock()
	go t.runLoop()
}

func (t *trayController) runLoop() {
	for {
		t.mu.Lock()
		fn := t.rebuild
		t.mu.Unlock()
		if fn != nil {
			fn()
		}
		t.mu.Lock()
		if !t.pending || !t.ready || t.rebuild == nil {
			t.running = false
			t.mu.Unlock()
			return
		}
		t.pending = false
		t.mu.Unlock()
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
	if icon := icons.trayIconForPlatform(runtime.GOOS); icon.template {
		tray.SetTemplateIcon(icon.icon)
	} else {
		tray.SetIcon(icon.icon)
	}
	tray.SetTooltip(ApplicationName)
	if runtime.GOOS == "linux" {
		// Wails v3 Linux StatusNotifierItem 暂未实现 setTooltip，其 D-Bus Title/Id/ToolTip 均取自 label（未设置时默认回退为 "Wails"）。
		tray.SetLabel(ApplicationName)
	}

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
				runAsync("cancel", func(context.Context) error { return vpnService.Cancel() })
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
