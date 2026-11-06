//go:build windows

package privilege

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err != nil {
		return fmt.Errorf("已有活动辅助会话，等待其退出后重试: %w", err)
	}
	return nil
}
