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

	var preHookCalledOnFail atomic.Bool
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
			preHookCalledOnFail.Store(true)
			return nil
		}),
	)

	_, err := checker.Apply(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected SHA-256 mismatch error, got: %v", err)
	}
	if preHookCalledOnFail.Load() {
		t.Fatal("preUpdateHook should not be called before download and SHA-256 verification succeed")
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
		preHookCalled      atomic.Bool
		progressEvents     atomic.Int32
		restartHandled     atomic.Bool
		finishedCalled     atomic.Int32
		finishedRestarting atomic.Bool
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
		WithUpdateFinishedHook(func(restarting bool) {
			finishedCalled.Add(1)
			finishedRestarting.Store(restarting)
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
	if finishedCalled.Load() != 1 || !finishedRestarting.Load() {
		t.Fatalf("expected updateFinishedHook(true) once, got count=%d restarting=%v", finishedCalled.Load(), finishedRestarting.Load())
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

	// 验证 preUpdateHook 返回错误时中止更新且不触发 updateFinishedHook
	var abortFinishedCalled atomic.Bool
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
		WithUpdateFinishedHook(func(_ bool) {
			abortFinishedCalled.Store(true)
		}),
	)
	if _, err := abortChecker.Apply(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "vpn disconnect failed") {
		t.Fatalf("expected preUpdate error, got %v", err)
	}
	if abortFinishedCalled.Load() {
		t.Fatal("expected updateFinishedHook not to be called when preUpdateHook fails")
	}
}

func TestCheckerApplyFallsBackToDirectWhenProxyChecksumMismatches(t *testing.T) {
	validPayload := []byte("authentic-release-archive")
	sumBytes := sha256.Sum256(validPayload)
	expectedSHA := hex.EncodeToString(sumBytes[:])

	var proxyCalls, directCalls atomic.Int32
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls.Add(1)
		// 模拟加速镜像返回被污染或截断的内容，应触发回退至官方直连源。
		_, _ = w.Write([]byte("corrupted-mirror-payload"))
	}))
	defer proxySrv.Close()

	var releaseSrv *httptest.Server
	releaseSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			directCalls.Add(1)
			_, _ = w.Write(validPayload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.2.2",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.2.2",
			"assets": [
				{
					"name": "tunnelkit-0.2.2-windows-amd64.zip",
					"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.2.2/tunnelkit-0.2.2-windows-amd64.zip",
					"size": %d,
					"digest": "sha256:%s"
				}
			]
		}`, releaseSrv.URL, len(validPayload), expectedSHA)
	}))
	defer releaseSrv.Close()

	var appliedBytes []byte
	checker := NewChecker(
		"0.2.1",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(proxySrv.URL+"/"),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
		WithApplier(func(_ context.Context, archivePath string, _ ProgressFunc) (func() error, error) {
			data, err := os.ReadFile(archivePath)
			if err != nil {
				return nil, err
			}
			appliedBytes = data
			return func() error { return nil }, nil
		}),
	)

	if _, err := checker.Apply(context.Background(), nil); err != nil {
		t.Fatalf("Apply should succeed by falling back to direct source after proxy checksum mismatch, got: %v", err)
	}
	if proxyCalls.Load() != 1 || directCalls.Load() != 1 {
		t.Fatalf("expected proxy=1 direct=1, got proxy=%d direct=%d", proxyCalls.Load(), directCalls.Load())
	}
	if !bytes.Equal(appliedBytes, validPayload) {
		t.Fatalf("applied payload = %q, want %q", appliedBytes, validPayload)
	}
}

func TestCheckerTwoStageDownloadInstallAndElevationCancelRetry(t *testing.T) {
	payload := []byte("two-stage-release-archive-payload")
	sumBytes := sha256.Sum256(payload)
	expectedSHA := hex.EncodeToString(sumBytes[:])

	var downloadHits atomic.Int32
	var releaseSrv *httptest.Server
	releaseSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			downloadHits.Add(1)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			_, _ = w.Write(payload)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.3.0",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.3.0",
			"assets": [{
				"name": "tunnelkit-0.3.0-windows-amd64.zip",
				"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.3.0/tunnelkit-0.3.0-windows-amd64.zip",
				"size": %d,
				"digest": "sha256:%s"
			}]
		}`, releaseSrv.URL, len(payload), expectedSHA)
	}))
	defer releaseSrv.Close()

	var (
		preHookCalls       atomic.Int32
		applierCalls       atomic.Int32
		finishedFalseCount atomic.Int32
		finishedTrueCount  atomic.Int32
		restarted          atomic.Bool
		failElevation      atomic.Bool
		installStages      []string
		installMessages    []string
	)
	failElevation.Store(true)

	checker := NewChecker(
		"0.2.0",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(""),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
		WithProgressListener(func(p Progress) {
			if !failElevation.Load() {
				installStages = append(installStages, p.Stage)
				installMessages = append(installMessages, p.Message)
			}
		}),
		WithPreUpdateHook(func(context.Context) error {
			preHookCalls.Add(1)
			return nil
		}),
		WithUpdateFinishedHook(func(isRestarting bool) {
			if isRestarting {
				finishedTrueCount.Add(1)
			} else {
				finishedFalseCount.Add(1)
			}
		}),
		WithApplier(func(_ context.Context, archivePath string, _ ProgressFunc) (func() error, error) {
			applierCalls.Add(1)
			if failElevation.Load() {
				return nil, ErrElevationCancelled
			}
			data, err := os.ReadFile(archivePath)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(data, payload) {
				return nil, fmt.Errorf("unexpected staged payload")
			}
			return func() error {
				restarted.Store(true)
				return nil
			}, nil
		}),
	)

	// Install before Download must fail with ErrUpdateNotReady.
	if err := checker.InstallAndRestart(context.Background()); !errors.Is(err, ErrUpdateNotReady) {
		t.Fatalf("InstallAndRestart before Download = %v, want ErrUpdateNotReady", err)
	}

	var stages []string
	info, err := checker.Download(context.Background(), func(p Progress) {
		stages = append(stages, p.Stage)
	})
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if info.LatestVersion != "0.3.0" || !checker.HasStagedUpdate() {
		t.Fatalf("expected staged update for 0.3.0, got info=%+v staged=%v", info, checker.HasStagedUpdate())
	}
	if preHookCalls.Load() != 0 {
		t.Fatal("Download must not invoke preUpdateHook or disrupt active VPN")
	}
	if len(stages) == 0 || stages[len(stages)-1] != StageReady {
		t.Fatalf("expected Download to end in StageReady, got %v", stages)
	}

	// Re-calling Download for the same version reuses verified archive with zero extra HTTP request.
	if _, err := checker.Download(context.Background(), nil); err != nil {
		t.Fatalf("second Download reuse failed: %v", err)
	}
	if downloadHits.Load() != 1 {
		t.Fatalf("expected staged reuse with 1 HTTP download, got %d", downloadHits.Load())
	}

	// Elevation cancellation preserves staged package so user can retry immediately without re-downloading.
	if err := checker.InstallAndRestart(context.Background()); !errors.Is(err, ErrElevationCancelled) {
		t.Fatalf("first InstallAndRestart = %v, want ErrElevationCancelled", err)
	}
	if !checker.HasStagedUpdate() || finishedFalseCount.Load() != 1 {
		t.Fatalf("elevation cancel must preserve staged archive and release VPN maintenance gate: staged=%v finishedFalse=%d", checker.HasStagedUpdate(), finishedFalseCount.Load())
	}

	// Retry install succeeds with zero re-download and cleans up staged archive.
	failElevation.Store(false)
	if err := checker.InstallAndRestart(context.Background()); err != nil {
		t.Fatalf("retry InstallAndRestart failed: %v", err)
	}
	if downloadHits.Load() != 1 || applierCalls.Load() != 2 || !restarted.Load() || finishedTrueCount.Load() != 1 {
		t.Fatalf("unexpected retry outcome: downloads=%d applier=%d restarted=%v finishedTrue=%d",
			downloadHits.Load(), applierCalls.Load(), restarted.Load(), finishedTrueCount.Load())
	}
	if checker.HasStagedUpdate() {
		t.Fatal("staged update must be cleared after successful install")
	}
	if len(installStages) != 2 || installStages[0] != StageInstalling || installStages[1] != StageRestarting {
		t.Fatalf("expected clean install stages [installing restarting], got %v (messages=%v)", installStages, installMessages)
	}
}

func TestCheckerDownloadCancelAndCleanupStaged(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseSrv *httptest.Server
	releaseSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			w.Header().Set("Content-Length", "1048576")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("partial-chunk"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"tag_name": "v0.3.1",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.3.1",
			"assets": [{
				"name": "tunnelkit-0.3.1-windows-amd64.zip",
				"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.3.1/tunnelkit-0.3.1-windows-amd64.zip",
				"size": 1048576,
				"digest": "sha256:%s"
			}]
		}`, releaseSrv.URL, strings.Repeat("a", 64))
	}))
	defer close(release)
	defer releaseSrv.Close()

	checker := NewChecker(
		"0.3.0",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(""),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
	)

	if checker.CancelDownload() {
		t.Fatal("CancelDownload must return false when idle")
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := checker.Download(context.Background(), nil)
		errCh <- err
	}()

	<-started
	if !checker.CancelDownload() {
		t.Fatal("CancelDownload must return true while download is active")
	}
	if err := <-errCh; !errors.Is(err, ErrUpdateCancelled) {
		t.Fatalf("Download after cancel = %v, want ErrUpdateCancelled", err)
	}
	if checker.HasStagedUpdate() {
		t.Fatal("cancelled download must not leave staged archive")
	}
}

func TestCheckerInstallDetectsStagedArchiveTamperingAndCleanup(t *testing.T) {
	payload := []byte("authentic-staged-bytes")
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
			"tag_name": "v0.3.2",
			"html_url": "https://github.com/maybk/tunnelkit/releases/tag/v0.3.2",
			"assets": [{
				"name": "tunnelkit-0.3.2-windows-amd64.zip",
				"browser_download_url": "%s/maybk/tunnelkit/releases/download/v0.3.2/tunnelkit-0.3.2-windows-amd64.zip",
				"size": %d,
				"digest": "sha256:%s"
			}]
		}`, releaseSrv.URL, len(payload), expectedSHA)
	}))
	defer releaseSrv.Close()

	var preHookCalled atomic.Bool
	checker := NewChecker(
		"0.3.1",
		WithHTTPClient(releaseSrv.Client()),
		WithReleaseURLs(releaseSrv.URL),
		WithProxyPrefix(""),
		WithPlatform("windows", "amd64"),
		WithAssetURLValidator(func(rawURL, _, _ string) bool {
			return strings.HasPrefix(rawURL, releaseSrv.URL+"/")
		}),
		WithPreUpdateHook(func(context.Context) error {
			preHookCalled.Store(true)
			return nil
		}),
	)

	if _, err := checker.Download(context.Background(), nil); err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	staged, ok := checker.snapshotStaged()
	if !ok {
		t.Fatal("expected staged archive")
	}
	if err := os.WriteFile(staged.archivePath, []byte("tampered-on-disk"), 0o600); err != nil {
		t.Fatalf("tamper write failed: %v", err)
	}

	if err := checker.InstallAndRestart(context.Background()); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("InstallAndRestart on tampered staged file = %v, want ErrChecksumMismatch", err)
	}
	if preHookCalled.Load() {
		t.Fatal("preUpdateHook must not run when secondary SHA-256 verification fails")
	}
	if checker.HasStagedUpdate() {
		t.Fatal("tampered staged update must be discarded")
	}
	if _, err := os.Stat(staged.tmpDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected tampered temp dir removed, stat err=%v", err)
	}

	// Re-download and verify CleanupStaged deletes temp directory cleanly.
	if _, err := checker.Download(context.Background(), nil); err != nil {
		t.Fatalf("second Download failed: %v", err)
	}
	staged, _ = checker.snapshotStaged()
	checker.CleanupStaged()
	if checker.HasStagedUpdate() {
		t.Fatal("CleanupStaged must clear staged state")
	}
	if _, err := os.Stat(staged.tmpDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CleanupStaged must remove temp dir, stat err=%v", err)
	}
}
