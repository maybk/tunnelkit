//go:build windows

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"

	"tunnelkit/internal/privilege"
)

func windowsCmdExePath() string {
	if sysDir, err := windows.GetSystemDirectory(); err == nil && sysDir != "" {
		return filepath.Join(sysDir, "cmd.exe")
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "cmd.exe")
}

func runElevatedInstaller(ctx context.Context, scriptPath, directory string) error {
	// 文件事务开始后不能取消等待并删除 staging，否则子进程可能仍在写入组件。
	code, err := privilege.RunElevatedWindows(context.WithoutCancel(ctx), windowsCmdExePath(), `/d /c ""`+scriptPath+`""`, directory)
	if err != nil {
		if errors.Is(err, privilege.ErrCancelled) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("Windows UAC 授权更新失败: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("管理员提权替换更新文件失败 (exit=%d)", code)
	}
	return nil
}
