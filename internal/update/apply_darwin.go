//go:build darwin

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"tunnelkit/internal/privilege"
)

func applyPlatformUpdate(ctx context.Context, archivePath string, _ ProgressFunc) (func() error, error) {
	selfExe, err := ResolveExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("定位当前应用路径失败: %w", err)
	}
	bundlePath, ok := ResolveMacAppBundlePath(selfExe)
	if !ok {
		return nil, ErrSelfUpdateNotReady
	}

	mountDir, err := os.MkdirTemp("", "tunnelkit-dmg-mount-*")
	if err != nil {
		return nil, fmt.Errorf("创建 DMG 挂载目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(mountDir)
	}()

	attachCmd := exec.CommandContext(
		ctx,
		"/usr/bin/hdiutil",
		"attach",
		"-nobrowse",
		"-readonly",
		"-mountpoint",
		mountDir,
		archivePath,
	)
	if out, err := attachCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("挂载更新镜像失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	defer func() {
		_ = exec.Command("/usr/bin/hdiutil", "detach", mountDir, "-force").Run()
	}()

	srcAppPath, err := findAppBundleInMount(mountDir)
	if err != nil {
		return nil, err
	}

	if err := replaceMacAppBundle(ctx, srcAppPath, bundlePath); err != nil {
		return nil, err
	}

	oldPID := os.Getpid()
	restart := func() error {
		return startMacRelaunch(oldPID, bundlePath)
	}
	return restart, nil
}

func startMacRelaunch(oldPID int, bundlePath string) error {
	if oldPID <= 1 {
		return fmt.Errorf("当前进程 PID 无效: %d", oldPID)
	}
	if strings.TrimSpace(bundlePath) == "" {
		return errors.New("应用包路径不能为空")
	}
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		buildMacRelaunchShellCmd(),
		"tunnelkit-relaunch",
		strconv.Itoa(oldPID),
		bundlePath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新版本应用失败: %w", err)
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func findAppBundleInMount(mountDir string) (string, error) {
	entries, err := os.ReadDir(mountDir)
	if err != nil {
		return "", fmt.Errorf("读取更新镜像内容失败: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
			continue
		}
		candidate := filepath.Join(mountDir, entry.Name())
		mainBin := filepath.Join(candidate, "Contents", "MacOS", "tunnelkit")
		helperBin := filepath.Join(candidate, "Contents", "MacOS", "tunnelkit-helper")
		if isExecutableFile(mainBin) && isExecutableFile(helperBin) {
			return candidate, nil
		}
	}
	return "", errors.New("更新镜像中未找到有效的 TunnelKit.app")
}

func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func replaceMacAppBundle(ctx context.Context, srcAppPath, dstAppPath string) error {
	backupPath := dstAppPath + ".bak"
	if needsMacElevation(dstAppPath) {
		return replaceMacAppBundleElevated(ctx, srcAppPath, dstAppPath, backupPath)
	}

	parentDir := filepath.Dir(dstAppPath)
	bundleName := filepath.Base(dstAppPath)
	stageDir, err := os.MkdirTemp(parentDir, "."+bundleName+".stage-*")
	if err != nil {
		if isMacPermissionError(err) {
			return replaceMacAppBundleElevated(ctx, srcAppPath, dstAppPath, backupPath)
		}
		return fmt.Errorf("创建更新暂存目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stageDir)
	}()

	stageAppPath := filepath.Join(stageDir, bundleName)
	if out, err := exec.CommandContext(ctx, "/usr/bin/ditto", srcAppPath, stageAppPath).CombinedOutput(); err != nil {
		return fmt.Errorf("准备新应用包失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	_ = exec.CommandContext(ctx, "/usr/bin/xattr", "-dr", "com.apple.quarantine", stageAppPath).Run()

	if err := swapMacAppBundles(stageAppPath, dstAppPath, backupPath); err != nil {
		if isMacPermissionError(err) && !errors.Is(err, ErrRollbackFailed) {
			return replaceMacAppBundleElevated(ctx, srcAppPath, dstAppPath, backupPath)
		}
		return err
	}

	touchBundle(dstAppPath)
	return nil
}

func needsMacElevation(dstAppPath string) bool {
	if os.Geteuid() == 0 {
		return false
	}
	parentDir := filepath.Dir(dstAppPath)
	if unix.Access(parentDir, unix.W_OK|unix.X_OK) != nil {
		return true
	}
	info, err := os.Lstat(dstAppPath)
	if err != nil {
		return !errors.Is(err, os.ErrNotExist)
	}
	if unix.Access(dstAppPath, unix.W_OK) != nil {
		return true
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return true
	}
	return false
}

func swapMacAppBundles(stageAppPath, dstAppPath, backupPath string) error {
	_ = os.RemoveAll(backupPath)

	swapErr := unix.RenamexNp(stageAppPath, dstAppPath, unix.RENAME_SWAP)
	if swapErr == nil {
		return nil
	}
	if isMacPermissionError(swapErr) {
		return fmt.Errorf("替换应用包权限不足: %w", os.ErrPermission)
	}
	if !shouldFallbackFromAtomicSwap(swapErr) {
		return fmt.Errorf("替换应用包失败: %w", swapErr)
	}

	return swapMacAppBundlesFallback(stageAppPath, dstAppPath, backupPath)
}

func swapMacAppBundlesFallback(stageAppPath, dstAppPath, backupPath string) error {
	_ = os.RemoveAll(backupPath)

	hadExisting := false
	if _, err := os.Lstat(dstAppPath); err == nil {
		if err := os.Rename(dstAppPath, backupPath); err != nil {
			return fmt.Errorf("备份原应用包失败: %w", err)
		}
		hadExisting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查原应用包失败: %w", err)
	}

	if err := os.Rename(stageAppPath, dstAppPath); err != nil {
		installErr := fmt.Errorf("替换应用包失败: %w", err)
		if hadExisting {
			if restoreErr := os.Rename(backupPath, dstAppPath); restoreErr != nil {
				return errors.Join(ErrRollbackFailed, installErr, restoreErr)
			}
		}
		return installErr
	}

	if hadExisting {
		_ = os.RemoveAll(backupPath)
	}
	return nil
}

func isMacPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission) ||
		errors.Is(err, unix.EACCES) ||
		errors.Is(err, unix.EPERM)
}

func shouldFallbackFromAtomicSwap(err error) bool {
	return errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EOPNOTSUPP) ||
		errors.Is(err, unix.EINVAL) ||
		errors.Is(err, unix.ENOENT) ||
		errors.Is(err, unix.EXDEV)
}

func touchBundle(bundlePath string) {
	now := time.Now()
	_ = os.Chtimes(bundlePath, now, now)
}

func replaceMacAppBundleElevated(ctx context.Context, srcAppPath, dstAppPath, backupPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	expectedSums := make(map[string]string, len(macVerifiedBundleBinaries))
	for _, rel := range macVerifiedBundleBinaries {
		sum, err := fileSHA256Hex(filepath.Join(srcAppPath, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("校验暂存更新文件失败: %w", err)
		}
		expectedSums[rel] = sum
	}
	shellCmd := buildMacElevatedUpdateShellCmd(srcAppPath, dstAppPath, backupPath, expectedSums)
	// 提权后的脚本必须完成替换或回滚，不能在文件事务中途中断。
	if err := privilege.RunElevatedDarwinShell(context.WithoutCancel(ctx), shellCmd); err != nil {
		if errors.Is(err, privilege.ErrCancelled) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("管理员授权安装更新失败: %w", err)
	}
	return nil
}
