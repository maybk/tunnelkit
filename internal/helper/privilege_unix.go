//go:build darwin || linux

package helper

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// EnsureRootIdentity synchronizes the real UID and GID to 0 when the process
// is launched via a setuid-root binary (where euid == 0 but ruid != 0).
// External system tools invoked by fortivpn/internal/netcfg (ifconfig, route,
// scutil, ip, resolvectl) require ruid == 0 to retain root privileges.
func EnsureRootIdentity() error {
	if os.Geteuid() != 0 {
		return errors.New("helper requires root privileges; reinstall the system helper")
	}
	if err := syscall.Setgroups(nil); err != nil {
		return fmt.Errorf("helper: clear groups: %w", err)
	}
	if os.Getgid() != 0 {
		if err := syscall.Setgid(0); err != nil {
			return fmt.Errorf("helper: setgid: %w", err)
		}
	}
	if os.Getuid() != 0 {
		if err := syscall.Setuid(0); err != nil {
			return fmt.Errorf("helper: setuid(0): %w", err)
		}
	}
	syscall.Umask(0o077)
	os.Clearenv()
	if err := os.Setenv("PATH", "/usr/sbin:/usr/bin:/sbin:/bin"); err != nil {
		return err
	}
	return os.Chdir("/")
}

// RunIfWindowsService returns false on POSIX platforms.
func RunIfWindowsService(_ string) (bool, error) {
	return false, nil
}
