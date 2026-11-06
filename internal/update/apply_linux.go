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
	if err := ReplaceFilesWithBackup(stageDir, targetDir, files, 0o755, true); err != nil {
		if errors.Is(err, os.ErrPermission) {
			if elevErr := replaceLinuxFilesElevated(ctx, stageDir, targetDir); elevErr != nil {
				return nil, elevErr
			}
		} else {
			return nil, fmt.Errorf("替换可执行文件失败: %w", err)
		}
	}

	if err := privilege.Ensure(ctx, filepath.Join(targetDir, "tunnelkit-helper")); err != nil {
		return nil, fmt.Errorf("应用已替换，辅助组件升级未完成: %w", err)
	}
	restart := func() error {
		return exec.Command(selfExe).Start()
	}
	return restart, nil
}

func replaceLinuxFilesElevated(ctx context.Context, stageDir, targetDir string) error {
	srcApp := filepath.Join(stageDir, "tunnelkit")
	srcHelper := filepath.Join(stageDir, "tunnelkit-helper")
	dstApp := filepath.Join(targetDir, "tunnelkit")
	dstHelper := filepath.Join(targetDir, "tunnelkit-helper")

	installBin, err := exec.LookPath("install")
	if err != nil {
		return err
	}
	shBin, err := exec.LookPath("sh")
	if err != nil {
		return err
	}
	script := `"$1" -m 0755 "$2" "$4" && "$1" -m 0755 "$3" "$4" && "$4/tunnelkit-helper" install`
	args := []string{shBin, "-c", script, "sh", installBin, srcApp, srcHelper, targetDir}
	if pkexec, err := exec.LookPath("pkexec"); err == nil {
		out, err := exec.CommandContext(ctx, pkexec, args...).CombinedOutput()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("管理员更新失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	if sudo, err := exec.LookPath("sudo"); err == nil {
		out, err := exec.CommandContext(ctx, sudo, append([]string{"-n"}, args...)...).CombinedOutput()
		if err == nil {
			return nil
		}
		return fmt.Errorf("管理员更新失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return fmt.Errorf("无权限写入 %s 与 %s，请使用包管理器更新", dstApp, dstHelper)
}
