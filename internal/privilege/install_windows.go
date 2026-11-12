//go:build windows

package privilege

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func componentPairs(source, target string) [][2]string {
	return [][2]string{
		{source, target},
		{filepath.Join(filepath.Dir(source), "wintun.dll"), filepath.Join(filepath.Dir(target), "wintun.dll")},
	}
}

func ensureProtectedDirectory(dir, sddl string) error {
	err := os.Mkdir(dir, 0o755)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := verifyNoLinkOrReparse(dir, true); err != nil {
			return err
		}
		trustedOwner, _, err := inspectSecurityDescriptor(dir, sddl)
		if err != nil {
			return err
		}
		if !trustedOwner {
			return fmt.Errorf("unsafe helper path owner: %s", dir)
		}
	} else if err := verifyNoLinkOrReparse(dir, true); err != nil {
		return err
	}
	if err := setACL(dir, sddl); err != nil {
		return err
	}
	return verifyPathACL(dir, sddl, true)
}

func protectDirectory(path string) error {
	installed, err := ExecutablePath()
	if err != nil {
		return err
	}
	state, err := stateRoot()
	if err != nil {
		return err
	}
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
		if err := ensureProtectedDirectory(dir, sddl); err != nil {
			return err
		}
	}
	return nil
}

func protectComponent(path string, _ os.FileMode) error {
	return setACL(path, binarySDDL)
}

func inspectInstalled(path string) (string, error) {
	elevated := windows.GetCurrentProcessToken().IsElevated()
	dir := filepath.Dir(path)
	if err := verifyNoLinkOrReparse(dir, true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "系统辅助目录未安装", nil
		}
		if !elevated && errors.Is(err, os.ErrPermission) {
			return "系统辅助目录访问权限待修复", nil
		}
		return "", err
	}
	trustedOwner, aclOK, err := inspectSecurityDescriptor(dir, directorySDDL)
	if err != nil {
		if !elevated && errors.Is(err, os.ErrPermission) {
			return "系统辅助目录安全描述符待修复", nil
		}
		return "", err
	}
	if !trustedOwner {
		return "", fmt.Errorf("unsafe helper path owner: %s", dir)
	}
	if !aclOK {
		return "系统辅助目录安全策略待更新", nil
	}

	for _, pair := range componentPairs(path, path) {
		comp := pair[1]
		if err := verifyNoLinkOrReparse(comp, false); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Sprintf("缺少组件 %s", filepath.Base(comp)), nil
			}
			if !elevated && errors.Is(err, os.ErrPermission) {
				return fmt.Sprintf("组件访问权限待修复 %s", filepath.Base(comp)), nil
			}
			return "", err
		}
		compTrusted, compACL, err := inspectSecurityDescriptor(comp, binarySDDL)
		if err != nil {
			if !elevated && errors.Is(err, os.ErrPermission) {
				return fmt.Sprintf("组件安全描述符待修复 %s", filepath.Base(comp)), nil
			}
			return "", err
		}
		if !compTrusted || !compACL {
			return fmt.Sprintf("组件安全策略待更新 %s", filepath.Base(comp)), nil
		}
	}
	return "", nil
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
