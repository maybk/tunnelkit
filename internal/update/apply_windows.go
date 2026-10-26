//go:build windows

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
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
		if errors.Is(err, os.ErrPermission) {
			if elevErr := replaceWindowsFilesElevated(ctx, stageDir, targetDir, files); elevErr != nil {
				return nil, elevErr
			}
		} else {
			return nil, fmt.Errorf("替换应用文件失败: %w", err)
		}
	}

	targetExe := filepath.Join(targetDir, "tunnelkit.exe")
	restart := func() error {
		return exec.Command(targetExe).Start()
	}
	return restart, nil
}

func replaceWindowsFilesElevated(ctx context.Context, stageDir, targetDir string, files []string) error {
	scriptPath := filepath.Join(stageDir, "apply-update.cmd")
	okSentinel := filepath.Join(stageDir, "done.ok")
	errSentinel := filepath.Join(stageDir, "done.err")

	var lines []string
	lines = append(lines, "@echo off")
	for _, name := range files {
		src := filepath.Join(stageDir, name)
		dst := filepath.Join(targetDir, name)
		old := dst + ".old"
		lines = append(lines, fmt.Sprintf(`del /f /q "%s" >nul 2>&1`, old))
		lines = append(lines, fmt.Sprintf(`if exist "%s" move /y "%s" "%s" >nul`, dst, dst, old))
		lines = append(lines, fmt.Sprintf(`copy /y "%s" "%s" >nul || (echo fail > "%s" & exit /b 1)`, src, dst, errSentinel))
	}
	lines = append(lines, fmt.Sprintf(`echo ok > "%s"`, okSentinel))
	content := strings.Join(lines, "\r\n") + "\r\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0o700); err != nil {
		return fmt.Errorf("生成提权更新脚本失败: %w", err)
	}

	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := windows.UTF16PtrFromString("cmd.exe")
	if err != nil {
		return err
	}
	paramPtr, err := windows.UTF16PtrFromString(`/c "` + scriptPath + `"`)
	if err != nil {
		return err
	}
	dirPtr, err := windows.UTF16PtrFromString(stageDir)
	if err != nil {
		return err
	}

	if err := windows.ShellExecute(0, verbPtr, filePtr, paramPtr, dirPtr, windows.SW_HIDE); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("已取消管理员授权，更新未安装")
		}
		return fmt.Errorf("Windows UAC 授权更新失败: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := os.Stat(okSentinel); err == nil {
			return nil
		}
		if _, err := os.Stat(errSentinel); err == nil {
			return errors.New("管理员提权替换更新文件失败")
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("等待管理员更新完成超时: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}
