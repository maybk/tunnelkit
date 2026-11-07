//go:build linux

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
		return nil, fmt.Errorf("定位当前可执行文件失败: %w", err)
	}
	targetDir := filepath.Dir(selfExe)

	stageDir, err := os.MkdirTemp("", "tunnelkit-linux-stage-*")
	if err != nil {
		return nil, fmt.Errorf("创建解压临时目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stageDir)
	}()

	required := map[string]os.FileMode{
		"tunnelkit":        0o755,
		"tunnelkit-helper": 0o755,
	}
	if err := ExtractTarGzTargets(archivePath, stageDir, required); err != nil {
		return nil, err
	}

	files := []string{"tunnelkit", "tunnelkit-helper"}
	if err := ReplaceFilesWithBackup(stageDir, targetDir, files, 0o755, false); err != nil {
		if errors.Is(err, os.ErrPermission) && !errors.Is(err, ErrRollbackFailed) {
			if elevErr := replaceLinuxFilesElevated(ctx, stageDir, targetDir); elevErr != nil {
				return nil, elevErr
			}
		} else {
			return nil, fmt.Errorf("替换可执行文件失败: %w", err)
		}
	}

	restart := func() error {
		return exec.Command(selfExe, restartArguments()...).Start()
	}
	return restart, nil
}

func replaceLinuxFilesElevated(ctx context.Context, stageDir, targetDir string) error {
	installBin, err := exec.LookPath("install")
	if err != nil {
		return fmt.Errorf("未找到 install 命令: %w", err)
	}
	script := buildPOSIXFileUpdate(stageDir, targetDir, []string{"tunnelkit", "tunnelkit-helper"}, installBin)
	if err := ctx.Err(); err != nil {
		return err
	}
	// 授权后的文件事务必须完成或回滚，不因 GUI 请求取消而中断在两次替换之间。
	if pkexecPath, pkErr := exec.LookPath("pkexec"); pkErr == nil {
		out, runErr := exec.Command(pkexecPath, "/bin/sh", "-c", script).CombinedOutput()
		if runErr == nil {
			return nil
		}
		text := strings.ToLower(strings.TrimSpace(string(out)))
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 126 {
			return ErrElevationCancelled
		}
		if strings.Contains(text, "dismissed") || strings.Contains(text, "cancel") {
			return ErrElevationCancelled
		}
		if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 127 {
			return fmt.Errorf("管理员更新失败: %s (%w)", strings.TrimSpace(string(out)), runErr)
		}
	}
	if sudoPath, sudoErr := exec.LookPath("sudo"); sudoErr == nil {
		out, runErr := exec.Command(sudoPath, "-n", "/bin/sh", "-c", script).CombinedOutput()
		if runErr == nil {
			return nil
		}
		return fmt.Errorf("管理员更新失败: %s (%w)", strings.TrimSpace(string(out)), runErr)
	}
	return fmt.Errorf("无权限写入 %s，请使用包管理器更新", targetDir)
}
