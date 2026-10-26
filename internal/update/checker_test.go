package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckerFallbackAndValidation(t *testing.T) {
	var proxyCalls, directCalls atomic.Int32

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := proxyCalls.Add(1)
		if n == 1 {
			// 第一次模拟加速源返回恶意钓鱼跳转地址，应被拒绝并触发回退。
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://evil.example.com/maybk/tunnelkit/releases/tag/v0.2.0"}`))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxySrv.Close()

	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.1","html_url":"https://github.com/maybk/tunnelkit/releases/tag/v0.1.1"}`))
	}))
	defer directSrv.Close()

	checker := NewChecker(
		"0.1.0",
		WithHTTPClient(directSrv.Client()),
		WithReleaseURLs(proxySrv.URL, directSrv.URL),
	)

	got, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !got.HasUpdate || got.CurrentVersion != "0.1.0" || got.LatestVersion != "0.1.1" {
		t.Fatalf("unexpected Info: %+v", got)
	}
	if got.ReleaseURL != "https://github.com/maybk/tunnelkit/releases/tag/v0.1.1" {
		t.Fatalf("unexpected ReleaseURL: %q", got.ReleaseURL)
	}
	if proxyCalls.Load() != 1 || directCalls.Load() != 1 {
		t.Fatalf("expected proxy=1 and direct=1 calls, got proxy=%d direct=%d", proxyCalls.Load(), directCalls.Load())
	}

	// 当前版本已是最新时，HasUpdate 应为 false。
	checkerUpToDate := NewChecker(
		"0.1.1",
		WithHTTPClient(directSrv.Client()),
		WithReleaseURLs(directSrv.URL),
	)
	upToDate, err := checkerUpToDate.Check(context.Background())
	if err != nil {
		t.Fatalf("Check up-to-date failed: %v", err)
	}
	if upToDate.HasUpdate || upToDate.LatestVersion != "0.1.1" {
		t.Fatalf("expected HasUpdate=false, got %+v", upToDate)
	}
}

func TestVersionAndReleaseURLHelpers(t *testing.T) {
	if !IsNewerVersion("0.1.0", "0.1.1") {
		t.Fatal("expected 0.1.1 > 0.1.0")
	}
	if !IsNewerVersion("0.1.0-rc.1", "0.1.0") {
		t.Fatal("expected stable 0.1.0 > prerelease 0.1.0-rc.1")
	}
	if IsNewerVersion("0.1.0", "0.1.0-rc.1") {
		t.Fatal("expected prerelease 0.1.0-rc.1 not newer than stable 0.1.0")
	}
	if IsNewerVersion("0.2.0", "0.1.9") {
		t.Fatal("expected 0.1.9 not newer than 0.2.0")
	}

	if !IsValidReleaseURL("https://github.com/maybk/tunnelkit/releases/tag/v0.1.1", "v0.1.1") {
		t.Fatal("expected valid release URL to pass")
	}
	invalidURLs := []string{
		"http://github.com/maybk/tunnelkit/releases/tag/v0.1.1",
		"https://github.com/other/tunnelkit/releases/tag/v0.1.1",
		"https://github.com/maybk/tunnelkit/releases/tag/v0.1.2",
		"https://github.com/maybk/tunnelkit/releases/tag/v0.1.1?redirect=1",
		"https://github.com/maybk/tunnelkit/releases/tag/v0.1.1#frag",
	}
	for _, u := range invalidURLs {
		if IsValidReleaseURL(u, "v0.1.1") {
			t.Fatalf("expected invalid release URL %q to be rejected", u)
		}
	}

	validAssetURL := "https://github.com/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-macos-arm64.dmg"
	if !IsValidAssetDownloadURL(validAssetURL, "v0.2.1", "tunnelkit-0.2.1-macos-arm64.dmg") {
		t.Fatal("expected valid asset download URL to pass")
	}
	invalidAssetURLs := []string{
		"http://github.com/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-macos-arm64.dmg",
		"https://evil.com/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-macos-arm64.dmg",
		"https://github.com/maybk/tunnelkit/releases/download/v0.2.0/tunnelkit-0.2.1-macos-arm64.dmg",
		"https://github.com/maybk/tunnelkit/releases/download/v0.2.1/other.dmg",
		validAssetURL + "?token=1",
	}
	for _, u := range invalidAssetURLs {
		if IsValidAssetDownloadURL(u, "v0.2.1", "tunnelkit-0.2.1-macos-arm64.dmg") {
			t.Fatalf("expected invalid asset download URL %q to be rejected", u)
		}
	}

	if got := ExpectedAssetNames("0.2.1", "darwin", "amd64"); len(got) != 2 || got[0] != "tunnelkit-0.2.1-macos-amd64.dmg" {
		t.Fatalf("unexpected darwin amd64 asset names: %v", got)
	}
	if got := ExpectedAssetNames("0.2.1", "windows", "arm64"); len(got) != 1 || got[0] != "tunnelkit-0.2.1-windows-arm64.zip" {
		t.Fatalf("unexpected windows arm64 asset names: %v", got)
	}
	if got := ExpectedAssetNames("0.2.1", "linux", "arm64"); len(got) != 1 || got[0] != "tunnelkit-0.2.1-linux-arm64.tar.gz" {
		t.Fatalf("unexpected linux arm64 asset names: %v", got)
	}
	if got := ExpectedAssetNames("0.2.1", "freebsd", "amd64"); got != nil {
		t.Fatalf("expected nil for unsupported platform, got %v", got)
	}

	sum := strings.Repeat("d", 64)
	if got, ok := ParseSHA256Digest("SHA256:" + strings.ToUpper(sum)); !ok || got != sum {
		t.Fatalf("unexpected ParseSHA256Digest: %q (ok=%v)", got, ok)
	}
	if got, ok := ParseSHA256Digest("invalid"); ok || got != "" {
		t.Fatalf("expected empty digest for invalid input, got %q (ok=%v)", got, ok)
	}
	if got, ok := ParseSHA256Sums("# comment\n"+sum+" *pkg.zip\n", "pkg.zip"); !ok || got != sum {
		t.Fatalf("unexpected ParseSHA256Sums: %q (ok=%v)", got, ok)
	}
}

func TestCheckerResolvesPlatformAssetsAndSHA256SumsFallback(t *testing.T) {
	dmgSum := strings.Repeat("a", 64)
	winSum := strings.Repeat("b", 64)
	linuxSum := strings.Repeat("c", 64)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maybk/tunnelkit/releases/download/v0.2.1/SHA256SUMS.txt" {
			_, _ = fmt.Fprintf(w, "%s  tunnelkit-0.2.1-linux-amd64.tar.gz\n", linuxSum)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.2.1",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.2.1",
			"assets": [
				{
					"name": "tunnelkit-0.2.1-macos-arm64.dmg",
					"browser_download_url": "https://github.com/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-macos-arm64.dmg",
					"size": 9175575,
					"digest": "sha256:%s"
				},
				{
					"name": "tunnelkit-0.2.1-windows-amd64.zip",
					"browser_download_url": "https://github.com/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-windows-amd64.zip",
					"size": 7824646,
					"digest": "sha256:%s"
				},
				{
					"name": "tunnelkit-0.2.1-linux-amd64.tar.gz",
					"browser_download_url": "https://github.com/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-linux-amd64.tar.gz",
					"size": 7683232,
					"digest": ""
				},
				{
					"name": "SHA256SUMS.txt",
					"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.2.1/SHA256SUMS.txt",
					"size": 128
				}
			]
		}`, dmgSum, winSum, srv.URL)
	}))
	defer srv.Close()

	allowTestAssets := WithAssetURLValidator(func(rawURL, _, _ string) bool {
		return strings.HasPrefix(rawURL, "https://github.com/maybk/tunnelkit/releases/download/") ||
			strings.HasPrefix(rawURL, srv.URL+"/")
	})

	macChecker := NewChecker(
		"0.2.0",
		WithHTTPClient(srv.Client()),
		WithReleaseURLs(srv.URL),
		WithPlatform("darwin", "arm64"),
		allowTestAssets,
	)
	macInfo, err := macChecker.Check(context.Background())
	if err != nil {
		t.Fatalf("mac Check failed: %v", err)
	}
	if !macInfo.CanSelfUpdate || macInfo.AssetName != "tunnelkit-0.2.1-macos-arm64.dmg" || macInfo.AssetSHA256 != dmgSum {
		t.Fatalf("unexpected macInfo: %+v", macInfo)
	}

	winChecker := NewChecker(
		"0.2.0",
		WithHTTPClient(srv.Client()),
		WithReleaseURLs(srv.URL),
		WithPlatform("windows", "amd64"),
		allowTestAssets,
	)
	winInfo, err := winChecker.Check(context.Background())
	if err != nil {
		t.Fatalf("win Check failed: %v", err)
	}
	if !winInfo.CanSelfUpdate || winInfo.AssetName != "tunnelkit-0.2.1-windows-amd64.zip" || winInfo.AssetSHA256 != winSum {
		t.Fatalf("unexpected winInfo: %+v", winInfo)
	}

	linuxChecker := NewChecker(
		"0.2.0",
		WithHTTPClient(srv.Client()),
		WithReleaseURLs(srv.URL),
		WithPlatform("linux", "amd64"),
		allowTestAssets,
	)
	linuxInfo, err := linuxChecker.Check(context.Background())
	if err != nil {
		t.Fatalf("linux Check failed: %v", err)
	}
	if !linuxInfo.CanSelfUpdate || linuxInfo.AssetName != "tunnelkit-0.2.1-linux-amd64.tar.gz" || linuxInfo.AssetSHA256 != linuxSum {
		t.Fatalf("unexpected linuxInfo (SHA256SUMS fallback): %+v", linuxInfo)
	}
}
