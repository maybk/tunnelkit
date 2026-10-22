package update

import (
	"context"
	"net/http"
	"net/http/httptest"
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
}
