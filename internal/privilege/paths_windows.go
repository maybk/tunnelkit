//go:build windows

package privilege

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func ExecutablePath() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	return filepath.Join(base, "TunnelKit Helper", "tunnelkit-helper.exe"), err
}

func stateRoot() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	return filepath.Join(base, "TunnelKit Helper"), err
}

func validateOwner(owner string) error {
	sid, err := windows.StringToSid(owner)
	if err != nil || sid.String() != owner {
		return errors.New("invalid desktop user SID")
	}
	// Only real OS user profiles can own a desktop session, never group SIDs.
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+owner, registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("resolve desktop user profile: %w", err)
	}
	defer key.Close()
	if _, _, err := key.GetStringValue("ProfileImagePath"); err != nil {
		return fmt.Errorf("resolve desktop user profile: %w", err)
	}
	return nil
}

func ValidateEndpoint(owner, endpoint string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	const prefix = `\\.\pipe\TunnelKit-`
	if !strings.HasPrefix(endpoint, prefix) {
		return errors.New("invalid helper pipe name")
	}
	value := strings.TrimPrefix(endpoint, prefix)
	if len(value) != 32 {
		return errors.New("invalid helper pipe identifier")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return errors.New("invalid helper pipe identifier")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("helper requires administrator privileges")
	}
	return nil
}

// Named pipes need no per-user filesystem state before authentication.
func PrepareEndpoint(owner string) error {
	return validateOwner(owner)
}
