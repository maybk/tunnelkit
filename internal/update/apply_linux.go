//go:build linux

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"tunnelkit/internal/privilege"
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
	if err := ctx.Err(); err != nil {
		return err
	}
	script := buildPOSIXFileUpdateWithHelperSync(
		stageDir,
		targetDir,
		[]string{"tunnelkit", "tunnelkit-helper"},
		installBin,
		filepath.Join(targetDir, "tunnelkit-helper"),
	)
	// 授权后的文件事务必须完成或回滚，不因 GUI 请求取消而中断在两次替换之间。
	if err := privilege.RunElevatedLinux(context.WithoutCancel(ctx), "/bin/sh", "-c", script); err != nil {
		if errors.Is(err, privilege.ErrCancelled) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("管理员更新失败: %w", err)
	}
	return nil
}
