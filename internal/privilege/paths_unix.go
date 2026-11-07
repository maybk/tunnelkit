//go:build darwin || linux

package privilege

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

func PrepareEndpoint(owner string) error {
	dir, err := SessionDir(owner)
	if err != nil {
		return err
	}
	return protectDirectory(dir)
}
