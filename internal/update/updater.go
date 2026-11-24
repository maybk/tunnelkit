package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageReady       = "ready"
	StageInstalling  = "installing"
	StageRestarting  = "restarting"
)

// Progress 表示应用内自更新的实时阶段与进度。
type Progress struct {
	Stage           string `json:"stage"`
	DownloadedBytes int64  `json:"downloadedBytes"`
	TotalBytes      int64  `json:"totalBytes"`
	Percent         int    `json:"percent"`
	Message         string `json:"message"`
}

// ProgressFunc 用于接收自更新过程中的状态与进度通知。
type ProgressFunc func(Progress)

// ApplierFunc 将已通过 SHA-256 校验的安装包应用到当前系统，并返回重启新进程的函数。
type ApplierFunc func(ctx context.Context, archivePath string, report ProgressFunc) (restart func() error, err error)

type stagedUpdate struct {
	info        Info
	tmpDir      string
	archivePath string
	expectedSHA string
	size        int64
}

func (c *Checker) makeReporter(onProgress ProgressFunc) ProgressFunc {
	return func(p Progress) {
		if c.progressListener != nil {
			c.progressListener(p)
		}
		if onProgress != nil {
			onProgress(p)
		}
	}
}

// Download 执行阶段一：下载更新包并校验 SHA-256，完成后保留在私有临时目录等待安装。
// 下载阶段不中断活跃 VPN 连接，且支持通过 CancelDownload 随时取消。
func (c *Checker) Download(ctx context.Context, onProgress ProgressFunc) (Info, error) {
	dlCtx, cleanup, err := c.beginDownload(ctx)
	if err != nil {
		return Info{}, err
	}
	defer cleanup()

	report := c.makeReporter(onProgress)
	staged, err := c.ensureStaged(dlCtx, report)
	if err != nil {
		return Info{}, err
	}

	report(Progress{
		Stage:           StageReady,
		DownloadedBytes: staged.size,
		TotalBytes:      staged.size,
		Percent:         100,
		Message:         "可以开始安装了",
	})
	return staged.info, nil
}

// CancelDownload 取消正在进行的更新包下载；未处于下载阶段或已进入校验/安装临界区时返回 false。
func (c *Checker) CancelDownload() bool {
	c.mu.Lock()
	cancel := c.downloadCancel
	c.downloadCancel = nil
	c.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// HasStagedUpdate 返回当前是否已有已下载校验的待安装更新包。
func (c *Checker) HasStagedUpdate() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.staged != nil
}

// CleanupStaged 清理尚未安装的暂存更新目录，通常在应用退出前调用。
func (c *Checker) CleanupStaged() {
	c.CancelDownload()
	c.mu.Lock()
	if c.updating || c.staged == nil {
		c.mu.Unlock()
		return
	}
	tmpDir := c.staged.tmpDir
	c.staged = nil
	c.mu.Unlock()
	if tmpDir != "" {
		_ = os.RemoveAll(tmpDir)
	}
}

// InstallAndRestart 执行阶段二：对已暂存更新包做二次 SHA-256 校验、断开 VPN、原地替换并重启。
// 若管理员授权被取消或替换前失败，将保留已校验的暂存包以便用户直接重试而无需重新下载。
func (c *Checker) InstallAndRestart(ctx context.Context) error {
	if err := c.beginUpdate(); err != nil {
		return err
	}
	prepared, restarting := false, false
	defer func() { c.finishUpdate(prepared, restarting) }()

	staged, ok := c.snapshotStaged()
	if !ok {
		return ErrUpdateNotReady
	}
	report := c.makeReporter(nil)
	restartFn, err := c.runStagedInstall(ctx, staged, report, &prepared)
	if err != nil {
		return err
	}
	err = c.invokeRestart(restartFn)
	restarting = err == nil
	return err
}

// Apply 兼容单步入口：下载、校验 SHA-256 并执行平台原地更新，返回重启新进程的函数。
func (c *Checker) Apply(ctx context.Context, onProgress ProgressFunc) (func() error, error) {
	dlCtx, err := c.beginUpdateWithDownloadCancel(ctx)
	if err != nil {
		return nil, err
	}
	prepared := false
	defer func() { c.finishUpdate(prepared, false) }()

	report := c.makeReporter(onProgress)
	staged, err := c.ensureStaged(dlCtx, report)
	if err != nil {
		return nil, err
	}
	return c.runStagedInstall(ctx, staged, report, &prepared)
}

// ApplyAndRestart 兼容单步入口：若尚未下载则先下载校验，随后执行安装并重启。
func (c *Checker) ApplyAndRestart(ctx context.Context) error {
	dlCtx, err := c.beginUpdateWithDownloadCancel(ctx)
	if err != nil {
		return err
	}
	prepared, restarting := false, false
	defer func() { c.finishUpdate(prepared, restarting) }()

	report := c.makeReporter(nil)
	staged, err := c.ensureStaged(dlCtx, report)
	if err != nil {
		return err
	}
	restartFn, err := c.runStagedInstall(ctx, staged, report, &prepared)
	if err != nil {
		return err
	}
	err = c.invokeRestart(restartFn)
	restarting = err == nil
	return err
}

func (c *Checker) invokeRestart(restartFn func() error) error {
	if c.restartHandler != nil {
		return c.restartHandler(restartFn)
	}
	if restartFn != nil {
		return restartFn()
	}
	return nil
}

func (c *Checker) beginDownload(ctx context.Context) (context.Context, func(), error) {
	dlCtx, err := c.beginUpdateWithDownloadCancel(ctx)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		c.mu.Lock()
		cancel := c.downloadCancel
		c.downloadCancel = nil
		c.updating = false
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	return dlCtx, cleanup, nil
}

func (c *Checker) beginUpdateWithDownloadCancel(ctx context.Context) (context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updating {
		return nil, ErrUpdateInProgress
	}
	dlCtx, cancel := context.WithCancel(ctx)
	c.updating = true
	c.downloadCancel = cancel
	return dlCtx, nil
}

func (c *Checker) beginUpdate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updating {
		return ErrUpdateInProgress
	}
	c.updating = true
	c.downloadCancel = nil
	return nil
}

func (c *Checker) clearDownloadCancel() {
	c.mu.Lock()
	c.downloadCancel = nil
	c.mu.Unlock()
}

func (c *Checker) finishUpdate(prepared, restarting bool) {
	if prepared && c.updateFinishedHook != nil {
		c.updateFinishedHook(restarting)
	}
	c.mu.Lock()
	cancel := c.downloadCancel
	c.downloadCancel = nil
	c.updating = false
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Checker) snapshotStaged() (stagedUpdate, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.staged == nil {
		return stagedUpdate{}, false
	}
	return *c.staged, true
}

func (c *Checker) discardStaged(tmpDir string) {
	c.mu.Lock()
	if c.staged != nil && (tmpDir == "" || c.staged.tmpDir == tmpDir) {
		tmpDir = c.staged.tmpDir
		c.staged = nil
	}
	c.mu.Unlock()
	if tmpDir != "" {
		_ = os.RemoveAll(tmpDir)
	}
}

func (c *Checker) resolveInstallableInfo(ctx context.Context) (Info, string, error) {
	c.mu.Lock()
	var info Info
	var hasCached bool
	if c.cachedInfo != nil && c.cachedInfo.CanSelfUpdate {
		info = *c.cachedInfo
		hasCached = true
	}
	c.mu.Unlock()

	if !hasCached {
		checked, err := c.Check(ctx)
		if err != nil {
			return Info{}, "", err
		}
		info = checked
	}

	if !info.HasUpdate || !info.CanSelfUpdate || info.AssetURL == "" || info.AssetName == "" {
		return Info{}, "", ErrSelfUpdateNotReady
	}
	expectedSum, ok := ParseSHA256Digest(info.AssetSHA256)
	if !ok {
		return Info{}, "", ErrSelfUpdateNotReady
	}
	validateURL := c.assetURLValidator
	if validateURL == nil {
		validateURL = IsValidAssetDownloadURL
	}
	if !validateURL(info.AssetURL, "v"+info.LatestVersion, info.AssetName) {
		return Info{}, "", ErrSelfUpdateNotReady
	}
	return info, expectedSum, nil
}

func (c *Checker) ensureStaged(ctx context.Context, report ProgressFunc) (stagedUpdate, error) {
	info, expectedSum, err := c.resolveInstallableInfo(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return stagedUpdate{}, ErrUpdateCancelled
		}
		return stagedUpdate{}, err
	}

	// 若磁盘已有同版本暂存包，重新核验 SHA-256 通过后直接复用，避免重复下载。
	if existing, ok := c.snapshotStaged(); ok {
		if existing.info.LatestVersion == info.LatestVersion &&
			existing.info.AssetName == info.AssetName &&
			strings.EqualFold(existing.expectedSHA, expectedSum) {
			if err := verifyFileSHA256(existing.archivePath, expectedSum); err == nil {
				c.clearDownloadCancel()
				return existing, nil
			}
		}
		c.discardStaged(existing.tmpDir)
	}

	tmpDir, err := os.MkdirTemp("", "tunnelkit-update-*")
	if err != nil {
		return stagedUpdate{}, fmt.Errorf("创建更新临时目录失败: %w", err)
	}

	archivePath := filepath.Join(tmpDir, filepath.Base(info.AssetName))
	downloaded, err := c.downloadAndVerify(ctx, info, expectedSum, archivePath, report)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		if errors.Is(err, context.Canceled) {
			return stagedUpdate{}, ErrUpdateCancelled
		}
		return stagedUpdate{}, err
	}
	c.clearDownloadCancel()

	size := max(downloaded, info.AssetSize)
	staged := stagedUpdate{
		info:        info,
		tmpDir:      tmpDir,
		archivePath: archivePath,
		expectedSHA: expectedSum,
		size:        size,
	}

	var prevTmpDir string
	c.mu.Lock()
	if c.staged != nil && c.staged.tmpDir != tmpDir {
		prevTmpDir = c.staged.tmpDir
	}
	copied := staged
	c.staged = &copied
	c.mu.Unlock()
	if prevTmpDir != "" {
		_ = os.RemoveAll(prevTmpDir)
	}
	return staged, nil
}

func (c *Checker) runStagedInstall(
	ctx context.Context,
	staged stagedUpdate,
	report ProgressFunc,
	prepared *bool,
) (func() error, error) {
	c.clearDownloadCancel()

	report(Progress{
		Stage:           StageInstalling,
		DownloadedBytes: staged.size,
		TotalBytes:      staged.size,
		Percent:         100,
		Message:         "正在安装更新…",
	})

	// 防 TOCTOU：真正执行停机与替换前静默复验暂存归档 SHA-256，不倒退进度文案。
	if err := verifyFileSHA256(staged.archivePath, staged.expectedSHA); err != nil {
		c.discardStaged(staged.tmpDir)
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrUpdateNotReady
		}
		return nil, err
	}

	if c.preUpdateHook != nil {
		if err := c.preUpdateHook(ctx); err != nil {
			return nil, err
		}
		*prepared = true
	}

	installReport := func(p Progress) {
		if p.Stage == "" {
			p.Stage = StageInstalling
		}
		if p.TotalBytes <= 0 {
			p.DownloadedBytes = staged.size
			p.TotalBytes = staged.size
		}
		if p.Percent <= 0 {
			p.Percent = 100
		}
		report(p)
	}

	applier := c.applier
	if applier == nil {
		applier = applyPlatformUpdate
	}
	restartFn, err := applier(ctx, staged.archivePath, installReport)
	if err != nil {
		return nil, err
	}

	c.discardStaged(staged.tmpDir)
	report(Progress{
		Stage:           StageRestarting,
		DownloadedBytes: staged.size,
		TotalBytes:      staged.size,
		Percent:         100,
		Message:         "正在重启应用…",
	})
	return restartFn, nil
}
