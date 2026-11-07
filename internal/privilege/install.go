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

// Ensure 确保随包分发的辅助组件已安装且为最新版本，仅在首次安装或内容变化时触发一次系统授权。
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

// Current 检查受保护的系统辅助组件及平台依赖是否与当前应用内置版本完全一致。
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
