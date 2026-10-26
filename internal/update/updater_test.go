package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCheckerApplyDownloadsVerifiesAndApplies(t *testing.T) {
	payload := []byte("fake-release-archive-payload-for-self-update")
	sumBytes := sha256.Sum256(payload)
	expectedSHA := hex.EncodeToString(sumBytes[:])

	var proxyDownloadCalls, directDownloadCalls atomic.Int32
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyDownloadCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxySrv.Close()

	var releaseSrv *httptest.Server
	releaseSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tunnelkit-0.2.1-windows-amd64.zip") {
			directDownloadCalls.Add(1)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.2.1",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.2.1",
			"assets": [
				{
					"name": "tunnelkit-0.2.1-windows-amd64.zip",
					"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-windows-amd64.zip",
					"size": %d,
					"digest": "sha256:%s"
				}
			]
		}`, releaseSrv.URL, len(payload), expectedSHA)
	}))
	defer releaseSrv.Close()

	var appliedArchiveBytes []byte
	var restarted atomic.Bool
	var stages []string

	checker := NewChecker(
		"0.2.0",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(proxySrv.URL+"/"),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
		WithApplier(func(_ context.Context, archivePath string, report ProgressFunc) (func() error, error) {
			report(Progress{Stage: StageInstalling, Percent: 90, Message: "正在解压更新包..."})
			data, err := os.ReadFile(archivePath)
			if err != nil {
				return nil, err
			}
			appliedArchiveBytes = data
			return func() error {
				restarted.Store(true)
				return nil
			}, nil
		}),
	)

	restartFn, err := checker.Apply(context.Background(), func(p Progress) {
		stages = append(stages, p.Stage)
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !bytes.Equal(appliedArchiveBytes, payload) {
		t.Fatalf("applied archive content mismatch: got %q, want %q", appliedArchiveBytes, payload)
	}
	if proxyDownloadCalls.Load() != 1 || directDownloadCalls.Load() != 1 {
		t.Fatalf("expected proxy=1 and direct=1 download calls, got proxy=%d direct=%d", proxyDownloadCalls.Load(), directDownloadCalls.Load())
	}
	if len(stages) == 0 || stages[len(stages)-1] != StageRestarting {
		t.Fatalf("expected final stage %q, got stages=%v", StageRestarting, stages)
	}
	if err := restartFn(); err != nil {
		t.Fatalf("restartFn failed: %v", err)
	}
	if !restarted.Load() {
		t.Fatal("expected restartFn to mark restarted=true")
	}
}

func TestCheckerApplyRejectsSHA256Mismatch(t *testing.T) {
	payload := []byte("tampered-payload")
	wrongSHA := strings.Repeat("f", 64)

	var releaseSrv *httptest.Server
	releaseSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.2.1",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.2.1",
			"assets": [
				{
					"name": "tunnelkit-0.2.1-windows-amd64.zip",
					"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-windows-amd64.zip",
					"size": %d,
					"digest": "sha256:%s"
				}
			]
		}`, releaseSrv.URL, len(payload), wrongSHA)
	}))
	defer releaseSrv.Close()

	checker := NewChecker(
		"0.2.0",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(""),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
	)

	_, err := checker.Apply(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected SHA-256 mismatch error, got: %v", err)
	}
}

func TestCheckerApplyAndRestartHooksAndConcurrency(t *testing.T) {
	payload := []byte("valid-binary-archive")
	sumBytes := sha256.Sum256(payload)
	expectedSHA := hex.EncodeToString(sumBytes[:])

	var releaseSrv *httptest.Server
	releaseSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.2.1",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.2.1",
			"assets": [
				{
					"name": "tunnelkit-0.2.1-windows-amd64.zip",
					"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.2.1/tunnelkit-0.2.1-windows-amd64.zip",
					"size": %d,
					"digest": "sha256:%s"
				}
			]
		}`, releaseSrv.URL, len(payload), expectedSHA)
	}))
	defer releaseSrv.Close()

	var (
		preHookCalled  atomic.Bool
		progressEvents atomic.Int32
		restartHandled atomic.Bool
	)

	checker := NewChecker(
		"0.2.0",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(""),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
		WithPreUpdateHook(func(_ context.Context) error {
			preHookCalled.Store(true)
			return nil
		}),
		WithProgressListener(func(_ Progress) {
			progressEvents.Add(1)
		}),
		WithRestartHandler(func(restartFn func() error) error {
			restartHandled.Store(true)
			return restartFn()
		}),
		WithApplier(func(_ context.Context, _ string, _ ProgressFunc) (func() error, error) {
			return func() error { return nil }, nil
		}),
	)

	if err := checker.ApplyAndRestart(context.Background()); err != nil {
		t.Fatalf("ApplyAndRestart failed: %v", err)
	}
	if !preHookCalled.Load() {
		t.Fatal("expected preUpdateHook to be called")
	}
	if progressEvents.Load() == 0 {
		t.Fatal("expected progressListener to receive events")
	}
	if !restartHandled.Load() {
		t.Fatal("expected restartHandler to be called")
	}

	// 验证并发互斥锁保护
	checker.mu.Lock()
	checker.updating = true
	checker.mu.Unlock()
	if _, err := checker.Apply(context.Background(), nil); !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("expected ErrUpdateInProgress, got %v", err)
	}
	checker.mu.Lock()
	checker.updating = false
	checker.mu.Unlock()

	// 验证 preUpdateHook 返回错误时中止更新
	abortChecker := NewChecker(
		"0.2.0",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
		WithPreUpdateHook(func(_ context.Context) error {
			return errors.New("vpn disconnect failed")
		}),
	)
	if _, err := abortChecker.Apply(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "vpn disconnect failed") {
		t.Fatalf("expected preUpdate error, got %v", err)
	}
}
