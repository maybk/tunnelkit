// Package app 负责将 Wails 运行时、桌面窗口、菜单栏与领域服务装配起来。
package app

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"tunnelkit/internal/api"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/storage"
	"tunnelkit/internal/update"
	"tunnelkit/internal/vpn"
)

const (
	ApplicationName     = "TunnelKit"
	EventVPNState       = "vpn:state"
	EventUpdateProgress = "update:progress"
)

func init() {
	application.RegisterEvent[vpn.State](EventVPNState)
	application.RegisterEvent[update.Progress](EventUpdateProgress)
}

func loadVersion(assets fs.FS) (string, error) {
	raw, err := fs.ReadFile(assets, "build/config.yml")
	if err != nil {
		return "", fmt.Errorf("读取应用版本配置失败: %w", err)
	}
	inInfo := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inInfo = strings.HasPrefix(trimmed, "info:")
			continue
		}
		if inInfo && strings.HasPrefix(trimmed, "version:") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
			if idx := strings.Index(rest, "#"); idx >= 0 {
				rest = strings.TrimSpace(rest[:idx])
			}
			version := strings.Trim(rest, `"'`)
			if version != "" {
				return version, nil
			}
		}
	}
	return "", errors.New("build/config.yml 缺少 info.version")
}

// Run 装配领域服务、Wails 应用、主窗口与菜单栏，并阻塞运行直到应用退出。
func Run(assets embed.FS) error {
	version, err := loadVersion(assets)
	if err != nil {
		return err
	}
	icons, err := loadIcons(assets)
	if err != nil {
		return err
	}
	frontendFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		frontendFS = assets
	}

	rootDir, err := storage.DefaultRoot()
	if err != nil {
		return fmt.Errorf("初始化数据目录失败: %w", err)
	}

	update.CleanupOldArtifacts()

	profileStore := profile.NewStore(rootDir)
	vpnService := vpn.NewService(rootDir, profileStore, vpn.NewDefaultLauncher())

	var (
		mainWindow application.Window
		wailsApp   *application.App
	)
	tray := &trayController{}
	systemAPI := newSystemAPI(version, vpnService, func() *application.App {
		return wailsApp
	})

	wailsApp = application.New(application.Options{
		Name:        ApplicationName,
		Description: "Fortinet SSL-VPN 桌面客户端",
		Icon:        icons.appIcon,
		Services: []application.Service{
			application.NewService(api.NewProfileAPI(profileStore, vpnService, tray.refresh)),
			application.NewService(api.NewVPNAPI(vpnService)),
			application.NewService(systemAPI),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendFS),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.tunnelkit.app",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				showMainWindow(mainWindow, icons.appIcon)
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			ApplicationID: "com.tunnelkit.app",
			ProgramName:   "tunnelkit",
		},
	})

	mainWindow = createMainWindow(wailsApp)
	tray.init(wailsApp, mainWindow, vpnService, profileStore, icons)

	initialState := vpnService.State()
	var (
		listenerMu    sync.Mutex
		lastStatus    = initialState.Status
		lastProfileID = initialState.ProfileID
	)
	vpnService.SetListener(func(state vpn.State) {
		listenerMu.Lock()
		changed := state.Status != lastStatus || state.ProfileID != lastProfileID
		if changed {
			lastStatus = state.Status
			lastProfileID = state.ProfileID
		}
		listenerMu.Unlock()
		if changed {
			tray.refresh()
		}
		wailsApp.Event.Emit(EventVPNState, state)
	})

	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go vpnService.Recover(context.Background())
	})
	wailsApp.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		showMainWindow(mainWindow, icons.appIcon)
	})

	return wailsApp.Run()
}

func newSystemAPI(version string, vpnService *vpn.Service, getApp func() *application.App) *api.SystemAPI {
	return api.NewSystemAPI(
		version,
		update.WithPreUpdateHook(func(ctx context.Context) error {
			switch vpnService.State().Status {
			case vpn.StatusConnecting:
				vpnService.Cancel()
			case vpn.StatusConnected:
				return vpnService.Disconnect(ctx)
			}
			return nil
		}),
		update.WithProgressListener(func(p update.Progress) {
			if wailsApp := getApp(); wailsApp != nil {
				wailsApp.Event.Emit(EventUpdateProgress, p)
			}
		}),
		update.WithRestartHandler(func(restartFn func() error) error {
			if restartFn != nil {
				if err := restartFn(); err != nil {
					return err
				}
			}
			if wailsApp := getApp(); wailsApp != nil {
				go func() {
					time.Sleep(150 * time.Millisecond)
					wailsApp.Quit()
				}()
			}
			return nil
		}),
	)
}
