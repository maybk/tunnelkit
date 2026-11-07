//go:build darwin

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	restart := func() error {
		return exec.Command("/usr/bin/open", append([]string{"-n", bundlePath, "--args"}, restartArguments()...)...).Run()
	}
	return restart, nil
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
	_ = os.RemoveAll(backupPath)

	if err := os.Rename(dstAppPath, backupPath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return replaceMacAppBundleElevated(ctx, srcAppPath, dstAppPath, backupPath)
		}
		return fmt.Errorf("备份原应用包失败: %w", err)
	}

	if out, err := exec.CommandContext(ctx, "/usr/bin/ditto", srcAppPath, dstAppPath).CombinedOutput(); err != nil {
		removeErr := os.RemoveAll(dstAppPath)
		restoreErr := os.Rename(backupPath, dstAppPath)
		installErr := fmt.Errorf("写入新应用包失败: %s (%w)", strings.TrimSpace(string(out)), err)
		if restoreErr != nil {
			return errors.Join(ErrRollbackFailed, installErr, removeErr, restoreErr)
		}
		return errors.Join(installErr, removeErr)
	}

	_ = exec.CommandContext(ctx, "/usr/bin/xattr", "-dr", "com.apple.quarantine", dstAppPath).Run()
	_ = os.RemoveAll(backupPath)
	return nil
}

func replaceMacAppBundleElevated(ctx context.Context, srcAppPath, dstAppPath, backupPath string) error {
	shellCmd := buildMacElevatedUpdateShellCmd(srcAppPath, dstAppPath, backupPath)

	escaped := strings.ReplaceAll(shellCmd, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	script := `do shell script "` + escaped + `" with administrator privileges`

	if err := ctx.Err(); err != nil {
		return err
	}
	// 提权后的脚本必须完成替换或回滚，不能在文件事务中途中断。
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		lower := strings.ToLower(text)
		if strings.Contains(lower, "(-128)") || strings.Contains(lower, "canceled") || strings.Contains(lower, "cancelled") || strings.Contains(text, "用户已取消") {
			return ErrElevationCancelled
		}
		return fmt.Errorf("管理员授权安装更新失败: %s (%w)", text, err)
	}
	return nil
}

func buildMacElevatedUpdateShellCmd(srcAppPath, dstAppPath, backupPath string) string {
	srcQ := darwinShellQuote(srcAppPath)
	dstQ := darwinShellQuote(dstAppPath)
	bakQ := darwinShellQuote(backupPath)
	return strings.Join([]string{
		"set -eu",
		"/bin/rm -rf " + bakQ,
		"/bin/mv " + dstQ + " " + bakQ,
		"if /usr/bin/ditto " + srcQ + " " + dstQ + "; then",
		"  /usr/bin/xattr -dr com.apple.quarantine " + dstQ + " || true;",
		"  /bin/rm -rf " + bakQ + ";",
		"else",
		"  /bin/rm -rf " + dstQ + ";",
		"  /bin/mv " + bakQ + " " + dstQ + ";",
		"  exit 1;",
		"fi",
	}, "\n")
}

func darwinShellQuote(value string) string {
	return shellQuote(value)
}
