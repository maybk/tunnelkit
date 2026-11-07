//go:build darwin || linux

package privilege

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

func componentPairs(source, target string) [][2]string { return [][2]string{{source, target}} }

func checkProtected(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || info.Mode()&0o022 != 0 || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory {
		return fmt.Errorf("unsafe helper path permissions: %s", path)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("invalid helper component: %s", path)
	}
	return nil
}

func protectDirectory(path string) error {
	// Resolve system aliases such as macOS /var once; every descendant must be
	// root-owned and non-writable to ordinary users before proceeding.
	parent := filepath.Dir(path)
	if parent != path {
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			if err := protectDirectory(parent); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return err
		}
		if err := verifyParents(resolved); err != nil {
			return err
		}
	}
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := checkProtected(path, true); err != nil {
		return err
	}
	// Directory traversal must not depend on the installing user's umask.
	return os.Chmod(path, 0o755)
}

func verifyParents(path string) error {
	for {
		if err := checkProtected(path, true); err != nil {
			return err
		}
		next := filepath.Dir(path)
		if next == path {
			return nil
		}
		path = next
	}
}

func protectComponent(path string, mode os.FileMode) error {
	if err := os.Chown(path, 0, 0); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func verifyInstalled(path string) error {
	resolved, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := verifyParents(resolved); err != nil {
		return err
	}
	if err := checkProtected(path, false); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSetuid == 0 || info.Mode()&0o111 == 0 {
		return errors.New("installed helper authorization is incomplete; reinstall the helper")
	}
	return nil
}

// Install 将辅助程序稳定部署至受保护的系统目录并设置 root setuid 权限。
func Install(source string) error {
	if callerUID != 0 || os.Geteuid() != 0 {
		return errors.New("helper installation requires fresh administrator authorization")
	}
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	if err := protectDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	base, err := stateRoot()
	if err != nil {
		return err
	}
	if err := protectDirectory(base); err != nil {
		return err
	}
	lock, err := AcquireSession()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := copyComponent(source, target, os.ModeSetuid|0o755); err != nil {
		return err
	}
	return verifyInstalled(target)
}

// Uninstall 仅移除已安装的辅助程序二进制，保留恢复状态以便后续重装后仍可恢复中断会话。
func Uninstall() error {
	if callerUID != 0 || os.Geteuid() != 0 {
		return errors.New("helper removal requires administrator authorization")
	}
	lock, err := AcquireSession()
	if err != nil {
		return err
	}
	defer lock.Close()
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := os.Remove("/usr/libexec/tunnelkit"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
