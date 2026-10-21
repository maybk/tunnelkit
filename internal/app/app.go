// Package app 负责将 Wails 运行时、桌面窗口、菜单栏与领域服务装配起来。
package app

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/api"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/storage"
	"tunnelkit/internal/vpn"
)

const (
	ApplicationName = "TunnelKit"
	// Version 需与 build/config.yml 中的 info.version 保持一致。
	Version       = "0.1.0"
	EventVPNState = "vpn:state"
)

func init() {
	application.RegisterEvent[vpn.State](EventVPNState)
}

// Run 装配领域服务、Wails 应用、主窗口与菜单栏，并阻塞运行直到应用退出。
func Run(assets embed.FS) error {
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
			application.NewService(api.NewSystemAPI(Version)),
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
