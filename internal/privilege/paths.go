// Package privilege owns the protected helper installation and runtime paths.
// It has no dependency on the VPN engine, GUI, or user configuration store.
package privilege

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var ErrCancelled = errors.New("elevation cancelled by user")

// Ensure installs or upgrades the packaged component, with one OS authorization.
func Ensure(ctx context.Context, source string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	current, err := Current(source)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	if err := elevateInstall(ctx, source); err != nil {
		return err
	}
	current, err = Current(source)
	if err != nil {
		return err
	}
	if !current {
		return errors.New("辅助组件授权配置未完成，请重新安装应用")
	}
	return nil
}

const WindowsServiceName = "TunnelKitHelper"

// SessionDir returns the protected, per-user helper state directory.
func SessionDir(owner string) (string, error) {
	if err := validateOwner(owner); err != nil {
		return "", err
	}
	base, err := stateRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "run", owner), nil
}

// JournalPath derives a fixed path without creating per-user state.
func JournalPath(owner string) (string, error) {
	dir, err := SessionDir(owner)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "netcfg-journal.json"), nil
}

// PrepareSession creates only system-owned paths, never a caller-selected path.
func PrepareSession(owner string) (string, error) {
	dir, err := SessionDir(owner)
	if err != nil {
		return "", err
	}
	if err := protectDirectory(dir); err != nil {
		return "", err
	}
	return filepath.Join(dir, "netcfg-journal.json"), nil
}

// Current reports whether the protected installed helper matches the packaged
// helper and its platform dependencies. A permission or integrity error is fatal.
func Current(source string) (bool, error) {
	target, err := ExecutablePath()
	if err != nil {
		return false, err
	}
	if err := verifyInstalled(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	for _, pair := range componentPairs(source, target) {
		a, err := digest(pair[0])
		if err != nil {
			return false, err
		}
		b, err := digest(pair[1])
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if a != b {
			return false, nil
		}
	}
	return serviceCurrent()
}

func digest(path string) ([32]byte, error) {
	var sum [32]byte
	f, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return sum, err
	}
	if !info.Mode().IsRegular() {
		return sum, fmt.Errorf("not a regular helper component: %s", path)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// copyComponent atomically installs into an already-protected directory.
func copyComponent(source, target string, mode os.FileMode) error {
	if same, err := filepath.EvalSymlinks(source); err == nil && same == target {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("helper component is not a regular file")
	}
	out, err := os.CreateTemp(filepath.Dir(target), ".install-*")
	if err != nil {
		return err
	}
	temp := out.Name()
	defer os.Remove(temp)
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := protectComponent(temp, mode); err != nil {
		return err
	}
	return os.Rename(temp, target)
}
