//go:build windows

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func applyPlatformUpdate(ctx context.Context, archivePath string, _ ProgressFunc) (func() error, error) {
	selfExe, err := ResolveExecutablePath()
	if err != nil {
		return nil, fmt.Errorf("定位当前可执行文件失败: %w", err)
	}
	targetDir := filepath.Dir(selfExe)

	stageDir, err := os.MkdirTemp("", "tunnelkit-win-stage-*")
	if err != nil {
		return nil, fmt.Errorf("创建解压临时目录失败: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stageDir)
	}()

	required := map[string]os.FileMode{
		"tunnelkit.exe":        0o755,
		"tunnelkit-helper.exe": 0o755,
		"wintun.dll":           0o644,
	}
	if err := ExtractZipTargets(archivePath, stageDir, required); err != nil {
		return nil, err
	}

	files := []string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"}
	if err := ReplaceFilesWithBackup(stageDir, targetDir, files, 0o755, false); err != nil {
		if errors.Is(err, os.ErrPermission) && !errors.Is(err, ErrRollbackFailed) {
			if elevErr := replaceWindowsFilesElevated(ctx, stageDir, targetDir, files); elevErr != nil {
				return nil, elevErr
			}
		} else {
			return nil, fmt.Errorf("替换应用文件失败: %w", err)
		}
	}

	targetExe := filepath.Join(targetDir, "tunnelkit.exe")
	restart := func() error {
		return startDetachedProcess(targetExe, targetDir, restartArguments())
	}
	return restart, nil
}

func startDetachedProcess(exePath, workDir string, args []string) error {
	cmd := exec.Command(exePath, args...)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:    syscall.CREATE_NEW_PROCESS_GROUP,
		NoInheritHandles: true,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新版本应用失败: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func replaceWindowsFilesElevated(ctx context.Context, stageDir, targetDir string, files []string) error {
	expectedSums := make(map[string]string, len(files))
	for _, name := range files {
		sum, err := fileSHA256Hex(filepath.Join(stageDir, name))
		if err != nil {
			return fmt.Errorf("校验暂存更新文件失败: %w", err)
		}
		expectedSums[name] = sum
	}
	scriptPath := filepath.Join(stageDir, "apply-update.cmd")
	okSentinel := filepath.Join(stageDir, "done.ok")
	errSentinel := filepath.Join(stageDir, "done.err")

	content := buildWindowsFileUpdateWithHelperSync(
		stageDir,
		targetDir,
		files,
		okSentinel,
		errSentinel,
		filepath.Join(targetDir, "tunnelkit-helper.exe"),
		expectedSums,
	)
	if err := os.WriteFile(scriptPath, []byte(content), 0o700); err != nil {
		return fmt.Errorf("生成提权更新脚本失败: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	// 使用 targetDir 作为提权子进程工作目录，避免从普通用户可写的 stageDir 解析相对路径或 DLL。
	if err := runElevatedInstaller(ctx, scriptPath, targetDir); err != nil {
		return err
	}
	if _, err := os.Stat(okSentinel); err == nil {
		return nil
	}
	return errors.New("管理员提权替换更新文件失败")
}
