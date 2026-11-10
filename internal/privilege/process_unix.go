//go:build darwin || linux

package privilege

import (
	"errors"
	"syscall"
)

func isProcessAlive(pid int) (bool, error) {
	if int64(pid) > 1<<31-1 {
		return false, nil
	}
	// EPERM still confirms a process exists under a different identity.
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}
