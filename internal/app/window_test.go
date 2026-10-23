package app

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"

	"tunnelkit/internal/profile"
	"tunnelkit/internal/vpn"
)

func TestLoadIconsAndPlatformSelection(t *testing.T) {
	version, err := loadVersion(os.DirFS("../.."))
	if err != nil || version == "" {
		t.Fatalf("loadVersion failed: version=%q, err=%v", version, err)
	}

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

func TestTraySelectionAndConnectState(t *testing.T) {
	profiles := []profile.Profile{
		{ID: "p1", Name: "001"},
		{ID: "p2", Name: "002"},
	}

	if got := resolveSelectedProfileID(vpn.State{Status: vpn.StatusDisconnected}, nil, ""); got != "" {
		t.Fatalf("resolveSelectedProfileID with empty profiles = %q, want empty", got)
	}
	if got := resolveSelectedProfileID(vpn.State{Status: vpn.StatusDisconnected}, profiles, ""); got != "p1" {
		t.Fatalf("resolveSelectedProfileID default = %q, want p1", got)
	}
	if got := resolveSelectedProfileID(vpn.State{Status: vpn.StatusDisconnected}, profiles, "p2"); got != "p2" {
		t.Fatalf("resolveSelectedProfileID keep current = %q, want p2", got)
	}
	if got := resolveSelectedProfileID(vpn.State{Status: vpn.StatusConnected, ProfileID: "p2"}, profiles, "p1"); got != "p2" {
		t.Fatalf("resolveSelectedProfileID active state override = %q, want p2", got)
	}
	if got := resolveSelectedProfileID(vpn.State{Status: vpn.StatusDisconnected}, profiles, "deleted"); got != "p1" {
		t.Fatalf("resolveSelectedProfileID fallback when current deleted = %q, want p1", got)
	}

	cases := []struct {
		name        string
		state       vpn.State
		selectedID  string
		wantChecked bool
		wantEnabled bool
	}{
		{
			name:        "disconnected without profiles",
			state:       vpn.State{Status: vpn.StatusDisconnected},
			selectedID:  "",
			wantChecked: false,
			wantEnabled: false,
		},
		{
			name:        "disconnected with selected profile",
			state:       vpn.State{Status: vpn.StatusDisconnected},
			selectedID:  "p1",
			wantChecked: false,
			wantEnabled: true,
		},
		{
			name:        "connecting",
			state:       vpn.State{Status: vpn.StatusConnecting, ProfileID: "p1"},
			selectedID:  "p1",
			wantChecked: true,
			wantEnabled: true,
		},
		{
			name:        "connected",
			state:       vpn.State{Status: vpn.StatusConnected, ProfileID: "p1"},
			selectedID:  "p1",
			wantChecked: true,
			wantEnabled: true,
		},
		{
			name:        "disconnecting",
			state:       vpn.State{Status: vpn.StatusDisconnecting, ProfileID: "p1"},
			selectedID:  "p1",
			wantChecked: false,
			wantEnabled: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checked, enabled := trayConnectState(tc.state, tc.selectedID)
			if checked != tc.wantChecked || enabled != tc.wantEnabled {
				t.Fatalf("trayConnectState(%+v, %q) = (%v, %v), want (%v, %v)",
					tc.state, tc.selectedID, checked, enabled, tc.wantChecked, tc.wantEnabled)
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

func TestTrayControllerDefersRefreshUntilReady(t *testing.T) {
	var called atomic.Int32
	done := make(chan struct{}, 4)

	tray := &trayController{
		rebuild: func() {
			called.Add(1)
			done <- struct{}{}
		},
	}

	// 在 Wails 事件循环未就绪前调用 refresh，不得触发 rebuild（避免 nil impl panic）。
	tray.refresh()
	tray.refresh()
	if got := called.Load(); got != 0 {
		t.Fatalf("rebuild called %d times before markReady, want 0", got)
	}

	// 标记就绪后应自动合并执行一次挂起的刷新。
	tray.markReady()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for deferred rebuild after markReady")
	}
	if got := called.Load(); got != 1 {
		t.Fatalf("rebuild called %d times after markReady, want 1", got)
	}

	// 就绪后再次调用 refresh 应立即异步执行。
	tray.refresh()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for immediate rebuild after ready")
	}
	if got := called.Load(); got != 2 {
		t.Fatalf("rebuild called %d times after second refresh, want 2", got)
	}
}

func TestTrayControllerCoalescesConcurrentRefreshes(t *testing.T) {
	var (
		called    atomic.Int32
		inFlight  atomic.Int32
		maxActive atomic.Int32
	)
	blockFirst := make(chan struct{})
	firstEntered := make(chan struct{})
	done := make(chan struct{}, 8)

	tray := &trayController{
		ready: true,
		rebuild: func() {
			cur := inFlight.Add(1)
			for {
				prev := maxActive.Load()
				if cur <= prev || maxActive.CompareAndSwap(prev, cur) {
					break
				}
			}
			n := called.Add(1)
			if n == 1 {
				close(firstEntered)
				<-blockFirst
			}
			inFlight.Add(-1)
			done <- struct{}{}
		},
	}

	tray.refresh()
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first rebuild to start")
	}

	// 在第一次 rebuild 执行期间连续触发多次 refresh，应被合并为后续 1 次串行执行。
	tray.refresh()
	tray.refresh()
	tray.refresh()

	close(blockFirst)

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for rebuild #%d", i+1)
		}
	}

	if got := called.Load(); got != 2 {
		t.Fatalf("rebuild called %d times, want 2 (1 initial + 1 coalesced)", got)
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("max concurrent rebuilds = %d, want 1", got)
	}
}

func TestPlatformBuildMetadataUsesApplicationName(t *testing.T) {
	configBytes, err := os.ReadFile("../../build/config.yml")
	if err != nil {
		t.Fatalf("read build/config.yml: %v", err)
	}
	configText := string(configBytes)
	for _, want := range []string{
		`productName: "TunnelKit"`,
		`description: "TunnelKit"`,
		`comments: "TunnelKit"`,
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("build/config.yml must contain %s", want)
		}
	}

	winInfoBytes, err := os.ReadFile("../../build/windows/info.json")
	if err != nil {
		t.Fatalf("read build/windows/info.json: %v", err)
	}
	var winInfo struct {
		Info map[string]struct {
			FileDescription string `json:"FileDescription"`
			ProductName     string `json:"ProductName"`
			Comments        string `json:"Comments"`
		} `json:"info"`
	}
	if err := json.Unmarshal(winInfoBytes, &winInfo); err != nil {
		t.Fatalf("unmarshal build/windows/info.json: %v", err)
	}
	if _, hasNeutral := winInfo.Info["0000"]; hasNeutral {
		t.Fatal("build/windows/info.json must not use neutral language block 0000")
	}
	enUS, ok := winInfo.Info["0409"]
	if !ok {
		t.Fatal("build/windows/info.json must define 0409 language block")
	}
	if enUS.FileDescription != ApplicationName {
		t.Fatalf("Windows FileDescription = %q, want %q", enUS.FileDescription, ApplicationName)
	}
	if enUS.ProductName != ApplicationName {
		t.Fatalf("Windows ProductName = %q, want %q", enUS.ProductName, ApplicationName)
	}

	for _, plistPath := range []string{"../../build/darwin/Info.plist", "../../build/darwin/Info.dev.plist"} {
		raw, err := os.ReadFile(plistPath)
		if err != nil {
			t.Fatalf("read %s: %v", plistPath, err)
		}
		text := string(raw)
		if !strings.Contains(text, "<key>CFBundleDisplayName</key>") || !strings.Contains(text, "<string>TunnelKit</string>") {
			t.Fatalf("%s must define CFBundleDisplayName = TunnelKit", plistPath)
		}
		if strings.Contains(text, "Cross-platform Fortinet SSL-VPN desktop client") {
			t.Fatalf("%s must not contain long English tagline", plistPath)
		}
	}
}
