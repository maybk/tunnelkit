package app

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/vpn"
)

func TestLoadIconsAndPlatformSelection(t *testing.T) {
	loaded, err := loadIcons(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("loadIcons failed: %v", err)
	}
	if len(loaded.trayTemplateIcon) == 0 {
		t.Fatal("trayTemplateIcon must not be empty")
	}
	if bytes.Equal(loaded.trayTemplateIcon, icons.SystrayMacTemplate) {
		t.Fatal("trayTemplateIcon must not use the default Wails 'W' template icon")
	}
	img, err := png.Decode(bytes.NewReader(loaded.trayTemplateIcon))
	if err != nil {
		t.Fatalf("trayTemplateIcon must be a valid PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 64 || bounds.Dy() != 64 {
		t.Fatalf("trayTemplateIcon size = %dx%d, want 64x64", bounds.Dx(), bounds.Dy())
	}
	visiblePixels := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x0800 {
				visiblePixels++
			}
		}
	}
	if visiblePixels < 1000 {
		t.Fatalf("trayTemplateIcon visiblePixels = %d, want >= 1000", visiblePixels)
	}

	appImg, err := png.Decode(bytes.NewReader(loaded.appIcon))
	if err != nil {
		t.Fatalf("appIcon must be a valid PNG: %v", err)
	}
	if appImg.Bounds().Dx() != 256 || appImg.Bounds().Dy() != 256 {
		t.Fatalf("appIcon size = %dx%d, want 256x256", appImg.Bounds().Dx(), appImg.Bounds().Dy())
	}
	minX, minY, maxX, maxY := 256, 256, 0, 0
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			if _, _, _, a := appImg.At(x, y).RGBA(); a > 0x8000 {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	bodyWidth := maxX - minX + 1
	bodyHeight := maxY - minY + 1
	if bodyWidth < 200 || bodyWidth > 212 || bodyHeight < 200 || bodyHeight > 212 {
		t.Fatalf("appIcon solid body = %dx%d (bbox %d,%d..%d,%d), want ~206x206 (Apple HIG 824/1024 safe zone)",
			bodyWidth, bodyHeight, minX, minY, maxX, maxY)
	}

	if icon, isTemplate := loaded.trayIconForPlatform("darwin"); !isTemplate || !bytes.Equal(icon, loaded.trayTemplateIcon) {
		t.Fatal("darwin must use trayTemplateIcon in template mode")
	}

	trayColorImg, err := png.Decode(bytes.NewReader(loaded.trayColorIcon))
	if err != nil {
		t.Fatalf("trayColorIcon must be a valid PNG: %v", err)
	}
	if trayColorImg.Bounds().Dx() != 64 || trayColorImg.Bounds().Dy() != 64 {
		t.Fatalf("trayColorIcon size = %dx%d, want 64x64", trayColorImg.Bounds().Dx(), trayColorImg.Bounds().Dy())
	}
	for _, goos := range []string{"windows", "linux"} {
		if icon, isTemplate := loaded.trayIconForPlatform(goos); isTemplate || !bytes.Equal(icon, loaded.trayColorIcon) {
			t.Fatalf("%s must use 64x64 full-color trayColorIcon in non-template mode", goos)
		}
	}
}

func TestTrayStatusLabelFormatsAllConnectionStates(t *testing.T) {
	cases := []struct {
		name  string
		state vpn.State
		want  string
	}{
		{
			name:  "disconnected",
			state: vpn.State{Status: vpn.StatusDisconnected},
			want:  "未连接",
		},
		{
			name:  "connecting",
			state: vpn.State{Status: vpn.StatusConnecting, ProfileName: "Corp"},
			want:  "正在连接 · Corp",
		},
		{
			name:  "reconnecting",
			state: vpn.State{Status: vpn.StatusConnecting, Phase: fortivpn.PhaseReconnecting, ProfileName: "Corp"},
			want:  "↻ 重新连接中 · Corp",
		},
		{
			name:  "connected",
			state: vpn.State{Status: vpn.StatusConnected, ProfileName: "Corp"},
			want:  "● 已连接 · Corp",
		},
		{
			name:  "disconnecting",
			state: vpn.State{Status: vpn.StatusDisconnecting, ProfileName: "Corp"},
			want:  "正在断开 · Corp",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trayStatusLabel(tc.state); got != tc.want {
				t.Fatalf("trayStatusLabel(%+v) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}

func TestMainWindowOptionsDefineUsableDesktopBounds(t *testing.T) {
	options := MainWindowOptions(false)

	if options.Title != ApplicationName {
		t.Fatalf("window title = %q, want %q", options.Title, ApplicationName)
	}
	if options.Width != DefaultWindowWidth || options.Height != DefaultWindowHeight {
		t.Fatalf("default window size = %dx%d, want %dx%d", options.Width, options.Height, DefaultWindowWidth, DefaultWindowHeight)
	}
	if options.MinWidth != MinWindowWidth || options.MinHeight != MinWindowHeight {
		t.Fatalf("minimum window size = %dx%d, want %dx%d", options.MinWidth, options.MinHeight, MinWindowWidth, MinWindowHeight)
	}
	if options.URL != "/" {
		t.Fatalf("window URL = %q, want /", options.URL)
	}
}

func TestMainWindowOptionsDefinePlatformTitleBarStrategies(t *testing.T) {
	macOptions := MainWindowOptionsForPlatform("darwin", true)

	if macOptions.Mac.TitleBar != application.MacTitleBarHiddenInset {
		t.Fatalf("macOS title bar = %v, want MacTitleBarHiddenInset", macOptions.Mac.TitleBar)
	}
	if macOptions.Mac.InvisibleTitleBarHeight != MacTitleBarHeightPoints {
		t.Fatalf("macOS invisible title bar height = %d, want %d", macOptions.Mac.InvisibleTitleBarHeight, MacTitleBarHeightPoints)
	}
	if macOptions.Frameless {
		t.Fatal("macOS must keep the native traffic-light frame")
	}

	windowsOptions := MainWindowOptionsForPlatform("windows", false)
	if !windowsOptions.Frameless {
		t.Fatal("Windows desktop window must be frameless")
	}
	if !windowsOptions.Windows.NonClientRegionSupport {
		t.Fatal("Windows non-client region support must be enabled")
	}
	if !windowsOptions.Windows.WebView2CompositionHosting {
		t.Fatal("Windows WebView2 composition hosting must be enabled")
	}
}
