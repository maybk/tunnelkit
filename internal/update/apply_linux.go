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
	if err := ReplaceFilesWithBackup(stageDir, targetDir, files, 0o755, true); err != nil {
		if errors.Is(err, os.ErrPermission) {
			if elevErr := replaceLinuxFilesElevated(ctx, stageDir, targetDir); elevErr != nil {
				return nil, elevErr
			}
		} else {
			return nil, fmt.Errorf("替换可执行文件失败: %w", err)
		}
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

	installArgs := []string{
		"-m", "0755",
		srcApp,
		srcHelper,
		targetDir,
	}
	if installBin, err := exec.LookPath("install"); err == nil {
		if pkexecPath, pkErr := exec.LookPath("pkexec"); pkErr == nil {
			args := append([]string{installBin}, installArgs...)
			out, runErr := exec.CommandContext(ctx, pkexecPath, args...).CombinedOutput()
			if runErr == nil {
				return nil
			}
			text := strings.ToLower(strings.TrimSpace(string(out)))
			if strings.Contains(text, "dismissed") || strings.Contains(text, "cancel") {
				return errors.New("已取消管理员授权，更新未安装")
			}
		}
		if sudoPath, sudoErr := exec.LookPath("sudo"); sudoErr == nil {
			args := append([]string{"-n", installBin}, installArgs...)
			if out, runErr := exec.CommandContext(ctx, sudoPath, args...).CombinedOutput(); runErr == nil {
				return nil
			} else {
				return fmt.Errorf("需要管理员权限更新 %s 与 %s: %s (%w)", dstApp, dstHelper, strings.TrimSpace(string(out)), runErr)
			}
		}
	}
	return fmt.Errorf("无权限写入 %s，请使用包管理器或管理员权限更新", targetDir)
}
