//go:build darwin || linux

package helperclient

import (
	"errors"
	"syscall"
)

func processPresent(pid int) (bool, error) {
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}
