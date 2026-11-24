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
	"strings"
	"time"
)

const (
	maxUpdateArchiveBytes = 300 << 20 // 300 MiB
	downloadTimeout       = 10 * time.Minute
	progressReportMinGap  = 80 * time.Millisecond
)

func verifyFileSHA256(filePath, expectedSum string) error {
	actualSum, err := fileSHA256Hex(filePath)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(actualSum)), []byte(strings.ToLower(expectedSum))) != 1 {
		return ErrChecksumMismatch
	}
	return nil
}

func (c *Checker) downloadAndVerify(
	parentCtx context.Context,
	info Info,
	expectedSum string,
	destPath string,
	report ProgressFunc,
) (int64, error) {
	urls := c.candidateDownloadURLs(info.AssetURL)
	var lastErr error
	var checksumErr error
	for _, endpoint := range urls {
		if err := parentCtx.Err(); err != nil {
			return 0, err
		}
		downloaded, err := c.downloadFromEndpoint(parentCtx, endpoint, info, expectedSum, destPath, report)
		if err == nil {
			return downloaded, nil
		}
		if errors.Is(err, context.Canceled) {
			_ = os.Remove(destPath)
			return 0, err
		}
		if errors.Is(err, ErrChecksumMismatch) {
			_ = os.Remove(destPath)
			checksumErr = err
			// 仅当官方源校验失败时立即中止；若加速源内容不匹配，自动回退至官方源重试。
			if endpoint == info.AssetURL {
				return 0, err
			}
			lastErr = err
			continue
		}
		lastErr = err
	}
	if lastErr != nil && errors.Is(lastErr, context.Canceled) {
		return 0, lastErr
	}
	if checksumErr != nil {
		return 0, checksumErr
	}
	return 0, ErrDownloadUpdate
}

func (c *Checker) downloadFromEndpoint(
	parentCtx context.Context,
	endpoint string,
	info Info,
	expectedSum string,
	destPath string,
	report ProgressFunc,
) (int64, error) {
	if report == nil {
		report = func(Progress) {}
	}
	ctx, cancel := context.WithTimeout(parentCtx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, ErrDownloadUpdate
	}
	req.Header.Set("User-Agent", fmt.Sprintf("TunnelKit/%s", info.CurrentVersion))

	resp, err := c.safeHTTPClient(downloadTimeout).Do(req)
	if err != nil {
		if parentCtx.Err() != nil {
			return 0, parentCtx.Err()
		}
		return 0, ErrDownloadUpdate
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, ErrDownloadUpdate
	}

	totalBytes := resp.ContentLength
	if totalBytes <= 0 {
		totalBytes = info.AssetSize
	}
	if totalBytes > maxUpdateArchiveBytes {
		return 0, ErrDownloadUpdate
	}

	outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, ErrDownloadUpdate
	}
	defer func() {
		_ = outFile.Close()
	}()

	report(Progress{
		Stage:           StageDownloading,
		DownloadedBytes: 0,
		TotalBytes:      totalBytes,
		Percent:         0,
		Message:         "正在下载更新…",
	})

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)
	limited := io.LimitReader(resp.Body, maxUpdateArchiveBytes+1)

	buf := make([]byte, 32*1024)
	var downloaded int64
	var lastReportBytes int64 = -1
	var lastReportTime time.Time

	for {
		n, readErr := limited.Read(buf)
		if n > 0 {
			downloaded += int64(n)
			if downloaded > maxUpdateArchiveBytes {
				return 0, ErrDownloadUpdate
			}
			if _, wErr := writer.Write(buf[:n]); wErr != nil {
				return 0, ErrDownloadUpdate
			}
			pct := 0
			if totalBytes > 0 {
				pct = min(int(downloaded*100/totalBytes), 100)
			}
			now := time.Now()
			if downloaded != lastReportBytes && (pct == 100 || now.Sub(lastReportTime) >= progressReportMinGap) {
				lastReportBytes = downloaded
				lastReportTime = now
				report(Progress{
					Stage:           StageDownloading,
					DownloadedBytes: downloaded,
					TotalBytes:      totalBytes,
					Percent:         pct,
					Message:         "正在下载更新…",
				})
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if parentCtx.Err() != nil {
				return 0, parentCtx.Err()
			}
			return 0, ErrDownloadUpdate
		}
	}

	if err := outFile.Close(); err != nil {
		return 0, ErrDownloadUpdate
	}

	finalTotal := max(totalBytes, downloaded)
	report(Progress{
		Stage:           StageVerifying,
		DownloadedBytes: downloaded,
		TotalBytes:      finalTotal,
		Percent:         100,
		Message:         "正在校验更新…",
	})

	actualSum := hex.EncodeToString(hasher.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(actualSum)), []byte(strings.ToLower(expectedSum))) != 1 {
		_ = os.Remove(destPath)
		return 0, ErrChecksumMismatch
	}
	return downloaded, nil
}
