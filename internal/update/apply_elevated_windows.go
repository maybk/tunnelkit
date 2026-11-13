//go:build windows

package update

import (
	"context"
	"errors"
	"fmt"

	"tunnelkit/internal/privilege"
)

func runElevatedInstaller(ctx context.Context, scriptPath, directory string) error {
	// 文件事务开始后不能取消等待并删除 staging，否则子进程可能仍在写入组件。
	code, err := privilege.RunElevatedWindows(context.WithoutCancel(ctx), "cmd.exe", `/d /c ""`+scriptPath+`""`, directory)
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
