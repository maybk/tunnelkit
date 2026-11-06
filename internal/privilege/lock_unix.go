//go:build darwin || linux

package privilege

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("已有活动辅助会话，等待其退出后重试: %w", err)
	}
	return nil
}
