//go:build windows

package privilege

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	binarySDDL    = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;BU)"
	directorySDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)"
	stateSDDL     = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
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

func componentPairs(source, target string) [][2]string {
	return [][2]string{
		{source, target},
		{filepath.Join(filepath.Dir(source), "wintun.dll"), filepath.Join(filepath.Dir(target), "wintun.dll")},
	}
}

// Compare canonical ACEs separately from descriptor bookkeeping flags (AI/AR).
func aclString(sd *windows.SECURITY_DESCRIPTOR) string {
	text := sd.String()
	if d := strings.Index(text, "D:"); d >= 0 {
		text = text[d:]
		if a := strings.Index(text, "("); a >= 0 {
			return text[a:]
		}
	}
	return ""
}

func verifyACL(path, sddl string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("helper paths must not be links")
	}
	attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path))
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("helper paths must not be reparse points")
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if !owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) && !owner.IsWellKnown(windows.WinLocalSystemSid) {
		return fmt.Errorf("unsafe helper path owner: %s", path)
	}
	want, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 || aclString(sd) != aclString(want) {
		return fmt.Errorf("unsafe helper path ACL: %s", path)
	}
	return nil
}

func setACL(path, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		dacl,
		nil,
	)
}

func protectDirectory(path string) error {
	installed, _ := ExecutablePath()
	state, _ := stateRoot()
	base := filepath.Dir(installed)
	sddl := directorySDDL
	if path == state || strings.HasPrefix(path, state+string(os.PathSeparator)) {
		base = state
		sddl = stateSDDL
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("invalid protected helper directory")
	}
	paths := []string{base}
	if rel != "." {
		for _, part := range strings.Split(rel, string(os.PathSeparator)) {
			paths = append(paths, filepath.Join(paths[len(paths)-1], part))
		}
	}
	for _, dir := range paths {
		err := os.Mkdir(dir, 0o755)
		if err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			if err := verifyACL(dir, sddl); err != nil {
				return err
			}
		} else if err := setACL(dir, sddl); err != nil {
			return err
		}
	}
	return nil
}

func protectComponent(path string, _ os.FileMode) error {
	return setACL(path, binarySDDL)
}

func verifyInstalled(path string) error {
	if err := verifyACL(filepath.Dir(path), directorySDDL); err != nil {
		return err
	}
	for _, pair := range componentPairs(path, path) {
		if err := verifyACL(pair[1], binarySDDL); err != nil {
			return err
		}
		info, err := os.Stat(pair[1])
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("invalid helper component")
		}
	}
	return nil
}

// Install must be called with an elevated token and after the SCM service stops.
func Install(source string) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("helper installation requires administrator authorization")
	}
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	if err := protectDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	state, err := stateRoot()
	if err != nil {
		return err
	}
	if err := protectDirectory(state); err != nil {
		return err
	}
	lock, err := AcquireSession()
	if err != nil {
		return err
	}
	defer lock.Close()
	for _, pair := range componentPairs(source, target) {
		if err := copyComponent(pair[0], pair[1], 0o755); err != nil {
			return err
		}
	}
	return verifyInstalled(target)
}

// Named pipes need no per-user filesystem state before authentication.
func PrepareEndpoint(owner string) error {
	return validateOwner(owner)
}
