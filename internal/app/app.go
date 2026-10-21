// Package app 负责将 Wails 运行时、桌面窗口、菜单栏与领域服务装配起来。
package app

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/api"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/storage"
	"tunnelkit/internal/vpn"
)

const (
	ApplicationName = "TunnelKit"
	EventVPNState   = "vpn:state"
)

func init() {
	application.RegisterEvent[vpn.State](EventVPNState)
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

	profileStore := profile.NewStore(rootDir)
	vpnService := vpn.NewService(rootDir, profileStore, vpn.NewDefaultLauncher())
	vpnService.Recover(context.Background())

	var mainWindow application.Window
	tray := &trayController{}
	wailsApp := application.New(application.Options{
		Name:        ApplicationName,
		Description: "Fortinet SSL-VPN 桌面客户端",
		Icon:        icons.appIcon,
		Services: []application.Service{
			application.NewService(api.NewProfileAPI(profileStore, vpnService, tray.refresh)),
			application.NewService(api.NewVPNAPI(vpnService)),
			application.NewService(api.NewSystemAPI(version)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendFS),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.tunnelkit.app",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if mainWindow != nil {
					application.InvokeAsync(func() {
						mainWindow.Show().Focus()
					})
				}
			},
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	mainWindow = createMainWindow(wailsApp)
	tray.init(wailsApp, mainWindow, vpnService, profileStore, icons)

	var (
		lastStatus      vpn.Status
		lastPhase       fortivpn.Phase
		lastProfileName string
	)
	vpnService.SetListener(func(state vpn.State) {
		if state.Status != lastStatus || state.Phase != lastPhase || state.ProfileName != lastProfileName {
			lastStatus = state.Status
			lastPhase = state.Phase
			lastProfileName = state.ProfileName
			tray.refresh()
		}
		wailsApp.Event.Emit(EventVPNState, state)
	})

	return wailsApp.Run()
}
