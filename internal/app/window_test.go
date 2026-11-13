package app

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
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
	// 44px = macOS 菜单栏 22pt 的 @2x 像素，Wails 按状态栏厚度设置模板图像尺寸。
	if bounds.Dx() != 44 || bounds.Dy() != 44 {
		t.Fatalf("trayTemplateIcon size = %dx%d, want 44x44", bounds.Dx(), bounds.Dy())
	}
	visiblePixels := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0x0800 {
				visiblePixels++
			}
			if a > 0x4000 && (r|g|b)>>8 > 2 {
				t.Fatalf("trayTemplateIcon pixel (%d,%d) must be black for template mode", x, y)
			}
		}
	}
	if visiblePixels < 400 {
		t.Fatalf("trayTemplateIcon visiblePixels = %d, want >= 400", visiblePixels)
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

	if icon := loaded.trayIconForPlatform("darwin"); !icon.template || !bytes.Equal(icon.icon, loaded.trayTemplateIcon) {
		t.Fatal("darwin must use trayTemplateIcon in template mode")
	}

	if icon := loaded.trayIconForPlatform("windows"); icon.template || !bytes.Equal(icon.icon, loaded.trayWindowsIcon) {
		t.Fatal("windows must use the full-color multi-frame tray ICO in non-template mode")
	}
	sizes := icoFrameSizes(t, loaded.trayWindowsIcon)
	for _, want := range []int{16, 20, 24, 32} {
		if !sizes[want] {
			t.Fatalf("windows tray ICO must contain a %dpx frame for SM_CXSMICON, got %v", want, sizes)
		}
	}

	trayColorImg, err := png.Decode(bytes.NewReader(loaded.trayColorIcon))
	if err != nil {
		t.Fatalf("trayColorIcon must be a valid PNG: %v", err)
	}
	if trayColorImg.Bounds().Dx() != 64 || trayColorImg.Bounds().Dy() != 64 {
		t.Fatalf("trayColorIcon size = %dx%d, want 64x64", trayColorImg.Bounds().Dx(), trayColorImg.Bounds().Dy())
	}
	if icon := loaded.trayIconForPlatform("linux"); icon.template || !bytes.Equal(icon.icon, loaded.trayColorIcon) {
		t.Fatal("linux must use 64x64 full-color trayColorIcon in non-template mode")
	}
}

// icoFrameSizes 解析 ICO 目录并返回其中包含的帧尺寸集合。
func icoFrameSizes(t *testing.T, data []byte) map[int]bool {
	t.Helper()
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		t.Fatal("tray icon must be a valid ICO file")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 || len(data) < 6+16*count {
		t.Fatal("tray ICO directory is truncated")
	}
	sizes := map[int]bool{}
	for i := range count {
		size := int(data[6+16*i])
		if size == 0 {
			size = 256
		}
		sizes[size] = true
	}
	return sizes
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
		fmt.Sprintf("productName: %q", ApplicationName),
		fmt.Sprintf("description: %q", ApplicationName),
		fmt.Sprintf("comments: %q", ApplicationName),
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

	wantPlistString := "<string>" + ApplicationName + "</string>"
	for _, plistPath := range []string{"../../build/darwin/Info.plist", "../../build/darwin/Info.dev.plist"} {
		raw, err := os.ReadFile(plistPath)
		if err != nil {
			t.Fatalf("read %s: %v", plistPath, err)
		}
		text := string(raw)
		if !strings.Contains(text, "<key>CFBundleDisplayName</key>") || !strings.Contains(text, wantPlistString) {
			t.Fatalf("%s must define CFBundleDisplayName = %s", plistPath, ApplicationName)
		}
		if strings.Contains(text, "Cross-platform Fortinet SSL-VPN desktop client") {
			t.Fatalf("%s must not contain long English tagline", plistPath)
		}
	}

	linuxTaskfileBytes, err := os.ReadFile("../../build/linux/Taskfile.yml")
	if err != nil {
		t.Fatalf("read build/linux/Taskfile.yml: %v", err)
	}
	linuxTaskfileText := string(linuxTaskfileBytes)
	wantDesktopNameFlag := fmt.Sprintf("-name %q", ApplicationName)
	if !strings.Contains(linuxTaskfileText, wantDesktopNameFlag) {
		t.Fatalf("build/linux/Taskfile.yml must pass %s to wails3 generate .desktop", wantDesktopNameFlag)
	}
	if !strings.Contains(linuxTaskfileText, `-comment "Fortinet SSL-VPN client"`) {
		t.Fatal(`build/linux/Taskfile.yml must pass -comment "Fortinet SSL-VPN client" to wails3 generate .desktop`)
	}

	linuxDesktopBytes, err := os.ReadFile("../../build/linux/desktop")
	if err != nil {
		t.Fatalf("read build/linux/desktop: %v", err)
	}
	linuxDesktopText := string(linuxDesktopBytes)
	if !strings.Contains(linuxDesktopText, "Name="+ApplicationName) {
		t.Fatalf("build/linux/desktop must define Name=%s", ApplicationName)
	}
	if !strings.Contains(linuxDesktopText, "Comment=Fortinet SSL-VPN client") {
		t.Fatal("build/linux/desktop must define Comment=Fortinet SSL-VPN client")
	}

	nfpmBytes, err := os.ReadFile("../../build/linux/nfpm/nfpm.yaml")
	if err != nil {
		t.Fatalf("read build/linux/nfpm/nfpm.yaml: %v", err)
	}
	if !strings.Contains(string(nfpmBytes), `description: "Fortinet SSL-VPN client"`) {
		t.Fatal(`build/linux/nfpm/nfpm.yaml must define description: "Fortinet SSL-VPN client"`)
	}
}

func TestWithSecurityHeadersInjectsCSPAndNosniff(t *testing.T) {
	handler := WithSecurityHeaders(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Security-Policy"); got != ContentSecurityPolicy {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, ContentSecurityPolicy)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
