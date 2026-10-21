package app

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	DefaultWindowWidth      = 960
	DefaultWindowHeight     = 640
	MinWindowWidth          = 780
	MinWindowHeight         = 520
	MacTitleBarHeightPoints = 52
)

func createMainWindow(wailsApp *application.App) application.Window {
	window := wailsApp.Window.NewWithOptions(MainWindowOptions(wailsApp.Env.IsDarkMode()))

	// 关闭按钮只隐藏窗口，应用继续驻留菜单栏，之后可从菜单栏重新打开。
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})
	return window
}

// MainWindowOptions 返回当前运行平台的主窗口配置。
func MainWindowOptions(darkMode bool) application.WebviewWindowOptions {
	return MainWindowOptionsForPlatform(runtime.GOOS, darkMode)
}

// MainWindowOptionsForPlatform 按平台声明标题栏融合策略：
// macOS 使用 HiddenInset 原生交通灯 + 48pt 透明标题栏；
// Windows 使用 Frameless + NonClientRegionSupport + WebView2CompositionHosting 原生命中测试。
func MainWindowOptionsForPlatform(platform string, darkMode bool) application.WebviewWindowOptions {
	// 与前端 --background 一致，避免 WebView 加载前在深色模式下闪白。
	background := application.NewRGB(244, 246, 250)
	if darkMode {
		background = application.NewRGB(13, 17, 28)
	}

	options := application.WebviewWindowOptions{
		Name:             "main",
		Title:            ApplicationName,
		Width:            DefaultWindowWidth,
		Height:           DefaultWindowHeight,
		MinWidth:         MinWindowWidth,
		MinHeight:        MinWindowHeight,
		URL:              "/",
		BackgroundColour: background,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: MacTitleBarHeightPoints,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	}

	if platform == "windows" {
		options.Frameless = true
		options.Windows.NonClientRegionSupport = true
		options.Windows.WebView2CompositionHosting = true
	}

	return options
}
