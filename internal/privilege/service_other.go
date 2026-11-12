//go:build !windows

package privilege

import (
	"errors"
	"runtime"
)

func checkService() (string, error) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		return "", nil
	}
	return "", errors.New("privileged helper is unsupported on this platform")
}

func InstallService(string) error {
	return errors.New("Windows service installation is unsupported on this platform")
}

func UninstallService() error {
	return errors.New("Windows service removal is unsupported on this platform")
}
