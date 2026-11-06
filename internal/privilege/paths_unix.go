//go:build darwin || linux

package privilege

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

var callerUID = os.Getuid()

func ExecutablePath() (string, error) {
	if runtime.GOOS == "darwin" {
		return "/Library/PrivilegedHelperTools/com.tunnelkit.helper", nil
	}
	return "/usr/libexec/tunnelkit/tunnelkit-helper", nil
}

func stateRoot() (string, error) {
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/TunnelKit", nil
	}
	return "/var/lib/tunnelkit", nil
}

func validateOwner(owner string) error {
	uid, err := strconv.Atoi(owner)
	if err != nil || uid < 0 || strconv.Itoa(uid) != owner {
		return errors.New("invalid helper owner UID")
	}
	return nil
}

// ValidateCaller binds setuid launches to the real user before identity changes.
func ValidateCaller(owner string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	if callerUID != 0 && owner != strconv.Itoa(callerUID) {
		return errors.New("helper owner differs from launching user")
	}
	if os.Geteuid() != 0 {
		return errors.New("helper requires root privileges; reinstall the helper in its system location")
	}
	return nil
}

func Endpoint(owner string) (string, error) {
	dir, err := SessionDir(owner)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "helper.sock"), nil
}

func ValidateEndpoint(owner, endpoint string) error {
	want, err := Endpoint(owner)
	if err != nil {
		return err
	}
	if endpoint != want {
		return errors.New("invalid helper IPC endpoint")
	}
	return ValidateCaller(owner)
}

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

// Install deploys a stable helper, rather than granting setuid to app-owned files.
func Install(source string) error {
	if callerUID != 0 || os.Geteuid() != 0 {
		return errors.New("helper installation requires fresh administrator authorization")
	}
	target, _ := ExecutablePath()
	if err := protectDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	base, _ := stateRoot()
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

// Uninstall removes only the installed executable. Recovery state is retained
// deliberately so an interrupted session remains recoverable after reinstall.
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

func serviceCurrent() (bool, error) { return true, nil }

func InstallService(string) error {
	return errors.New("Windows service installation is unsupported on POSIX")
}

func UninstallService() error {
	return errors.New("Windows service removal is unsupported on POSIX")
}

func PrepareEndpoint(owner string) error {
	dir, err := SessionDir(owner)
	if err != nil {
		return err
	}
	return protectDirectory(dir)
}
