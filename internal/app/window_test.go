package app

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

func TestTrayTemplateIconIsCustomEmbeddedPNG(t *testing.T) {
	if len(trayTemplateIcon) == 0 {
		t.Fatal("trayTemplateIcon must not be empty")
	}
	if bytes.Equal(trayTemplateIcon, icons.SystrayMacTemplate) {
		t.Fatal("trayTemplateIcon must not use the default Wails 'W' template icon")
	}
	img, err := png.Decode(bytes.NewReader(trayTemplateIcon))
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

	appImg, err := png.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		t.Fatalf("appIconPNG must be a valid PNG: %v", err)
	}
	if appImg.Bounds().Dx() != 256 || appImg.Bounds().Dy() != 256 {
		t.Fatalf("appIconPNG size = %dx%d, want 256x256", appImg.Bounds().Dx(), appImg.Bounds().Dy())
	}
	// Apple HIG 规范：1024x1024 画布内主体安全区为 824x824（256x256 下约 206x206，四周保留约 25px 透明边距），
	// 防止运行期 setApplicationIconImage 将图标铺满整个 Dock 格子。
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
		t.Fatalf("appIconPNG solid body = %dx%d (bbox %d,%d..%d,%d), want ~206x206 (Apple HIG 824/1024 safe zone)",
			bodyWidth, bodyHeight, minX, minY, maxX, maxY)
	}

	if icon, isTemplate := trayIconForPlatform("darwin"); !isTemplate || !bytes.Equal(icon, trayTemplateIcon) {
		t.Fatal("darwin must use trayTemplateIcon in template mode")
	}

	trayColorImg, err := png.Decode(bytes.NewReader(trayColorIcon))
	if err != nil {
		t.Fatalf("trayColorIcon must be a valid PNG: %v", err)
	}
	if trayColorImg.Bounds().Dx() != 64 || trayColorImg.Bounds().Dy() != 64 {
		t.Fatalf("trayColorIcon size = %dx%d, want 64x64", trayColorImg.Bounds().Dx(), trayColorImg.Bounds().Dy())
	}
	for _, goos := range []string{"windows", "linux"} {
		if icon, isTemplate := trayIconForPlatform(goos); isTemplate || !bytes.Equal(icon, trayColorIcon) {
			t.Fatalf("%s must use 64x64 full-color trayColorIcon in non-template mode", goos)
		}
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
