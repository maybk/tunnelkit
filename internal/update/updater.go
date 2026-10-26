package update

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	StageDownloading = "downloading"
	StageVerifying   = "verifying"
	StageInstalling  = "installing"
	StageRestarting  = "restarting"

	maxUpdateArchiveBytes = 300 << 20 // 300 MiB
	downloadTimeout       = 10 * time.Minute
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

// Apply 执行下载、SHA-256 校验与平台原地更新，并返回用于拉起新版本进程的重启函数。
func (c *Checker) Apply(ctx context.Context, onProgress ProgressFunc) (func() error, error) {
	c.mu.Lock()
	if c.updating {
		c.mu.Unlock()
		return nil, ErrUpdateInProgress
	}
	c.updating = true
	var info Info
	var hasCached bool
	if c.cachedInfo != nil && c.cachedInfo.CanSelfUpdate {
		info = *c.cachedInfo
		hasCached = true
	}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.updating = false
		c.mu.Unlock()
	}()

	report := func(p Progress) {
		if c.progressListener != nil {
			c.progressListener(p)
		}
		if onProgress != nil {
			onProgress(p)
		}
	}

	if !hasCached {
		checked, err := c.Check(ctx)
		if err != nil {
			return nil, err
		}
		info = checked
	}

	if !info.HasUpdate || !info.CanSelfUpdate || info.AssetURL == "" || info.AssetName == "" {
		return nil, ErrSelfUpdateNotReady
	}
	expectedSum, ok := ParseSHA256Digest(info.AssetSHA256)
	if !ok {
		return nil, ErrSelfUpdateNotReady
	}
	validateURL := c.assetURLValidator
	if validateURL == nil {
		validateURL = IsValidAssetDownloadURL
	}
	if !validateURL(info.AssetURL, "v"+info.LatestVersion, info.AssetName) {
		return nil, ErrSelfUpdateNotReady
	}

	if c.preUpdateHook != nil {
		if err := c.preUpdateHook(ctx); err != nil {
			return nil, err
		}
	}

	tmpDir, err := os.MkdirTemp("", "tunnelkit-update-*")
	if err != nil {
		return nil, fmt.Errorf("创建更新临时目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	archivePath := filepath.Join(tmpDir, filepath.Base(info.AssetName))
	if err := c.downloadAndVerify(ctx, info, expectedSum, archivePath, report); err != nil {
		return nil, err
	}

	report(Progress{
		Stage:           StageInstalling,
		DownloadedBytes: info.AssetSize,
		TotalBytes:      info.AssetSize,
		Percent:         100,
		Message:         "正在安装新版本…",
	})

	applier := c.applier
	if applier == nil {
		applier = applyPlatformUpdate
	}
	restartFn, err := applier(ctx, archivePath, report)
	if err != nil {
		return nil, err
	}

	report(Progress{
		Stage:           StageRestarting,
		DownloadedBytes: info.AssetSize,
		TotalBytes:      info.AssetSize,
		Percent:         100,
		Message:         "更新完成，正在重启应用…",
	})
	return restartFn, nil
}

// ApplyAndRestart 执行自更新并调用重启处理器（或默认重启函数）。
func (c *Checker) ApplyAndRestart(ctx context.Context) error {
	restartFn, err := c.Apply(ctx, nil)
	if err != nil {
		return err
	}
	if c.restartHandler != nil {
		return c.restartHandler(restartFn)
	}
	if restartFn != nil {
		return restartFn()
	}
	return nil
}

func (c *Checker) downloadAndVerify(
	parentCtx context.Context,
	info Info,
	expectedSum string,
	destPath string,
	report ProgressFunc,
) error {
	urls := c.candidateDownloadURLs(info.AssetURL)
	var lastErr error
	for _, endpoint := range urls {
		if err := parentCtx.Err(); err != nil {
			return err
		}
		err := c.downloadFromEndpoint(parentCtx, endpoint, info, expectedSum, destPath, report)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrChecksumMismatch) {
			return err
		}
		lastErr = err
	}
	if lastErr != nil && errors.Is(lastErr, context.Canceled) {
		return lastErr
	}
	return ErrDownloadUpdate
}

func (c *Checker) downloadFromEndpoint(
	parentCtx context.Context,
	endpoint string,
	info Info,
	expectedSum string,
	destPath string,
	report ProgressFunc,
) error {
	ctx, cancel := context.WithTimeout(parentCtx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ErrDownloadUpdate
	}
	req.Header.Set("User-Agent", fmt.Sprintf("TunnelKit/%s", info.CurrentVersion))

	// 下载大文件时复制 HTTP Client 并将总超时放宽至 downloadTimeout。
	baseClient := c.httpClient
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	dlClient := *baseClient
	if dlClient.Timeout > 0 && dlClient.Timeout < downloadTimeout {
		dlClient.Timeout = downloadTimeout
	}

	resp, err := dlClient.Do(req)
	if err != nil {
		if parentCtx.Err() != nil {
			return parentCtx.Err()
		}
		return ErrDownloadUpdate
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrDownloadUpdate
	}

	totalBytes := resp.ContentLength
	if totalBytes <= 0 {
		totalBytes = info.AssetSize
	}
	if totalBytes > maxUpdateArchiveBytes {
		return ErrDownloadUpdate
	}

	outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return ErrDownloadUpdate
	}
	defer func() {
		_ = outFile.Close()
	}()

	report(Progress{
		Stage:           StageDownloading,
		DownloadedBytes: 0,
		TotalBytes:      totalBytes,
		Percent:         0,
		Message:         "正在下载更新安装包…",
	})

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)
	limited := io.LimitReader(resp.Body, maxUpdateArchiveBytes+1)

	buf := make([]byte, 32*1024)
	var downloaded int64
	lastReportPct := -1
	var lastReportTime time.Time

	for {
		n, readErr := limited.Read(buf)
		if n > 0 {
			downloaded += int64(n)
			if downloaded > maxUpdateArchiveBytes {
				return ErrDownloadUpdate
			}
			if _, wErr := writer.Write(buf[:n]); wErr != nil {
				return ErrDownloadUpdate
			}
			pct := 0
			if totalBytes > 0 {
				pct = int(downloaded * 100 / totalBytes)
				if pct > 100 {
					pct = 100
				}
			}
			now := time.Now()
			if pct != lastReportPct && (pct == 100 || now.Sub(lastReportTime) >= 80*time.Millisecond) {
				lastReportPct = pct
				lastReportTime = now
				report(Progress{
					Stage:           StageDownloading,
					DownloadedBytes: downloaded,
					TotalBytes:      totalBytes,
					Percent:         pct,
					Message:         fmt.Sprintf("正在下载更新安装包 (%d%%)…", pct),
				})
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if parentCtx.Err() != nil {
				return parentCtx.Err()
			}
			return ErrDownloadUpdate
		}
	}

	if err := outFile.Close(); err != nil {
		return ErrDownloadUpdate
	}

	report(Progress{
		Stage:           StageVerifying,
		DownloadedBytes: downloaded,
		TotalBytes:      totalBytes,
		Percent:         100,
		Message:         "正在校验安装包 SHA-256 签名…",
	})

	actualSum := hex.EncodeToString(hasher.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(actualSum)), []byte(strings.ToLower(expectedSum))) != 1 {
		_ = os.Remove(destPath)
		return ErrChecksumMismatch
	}
	return nil
}
