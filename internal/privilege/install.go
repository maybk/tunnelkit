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

// Ensure 确保随包分发的辅助组件已安装且为最新版本，仅在首次安装、配置漂移或内容变化时触发一次系统授权。
func Ensure(ctx context.Context, source string) error {
	return ensureWith(ctx, source, inspectCurrent, elevateInstall)
}

func ensureWith(
	ctx context.Context,
	source string,
	inspectFn func(string) (string, error),
	elevateFn func(context.Context, string) error,
) error {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	reason, err := inspectFn(absSource)
	if err != nil {
		return err
	}
	if reason == "" {
		return nil
	}
	if err := elevateFn(ctx, absSource); err != nil {
		return err
	}
	reason, err = inspectFn(absSource)
	if err != nil {
		return err
	}
	if reason != "" {
		return fmt.Errorf("辅助组件授权配置未完成（%s），请重新安装应用", reason)
	}
	return nil
}

// Current 检查受保护的系统辅助组件及平台依赖是否与当前应用内置版本完全一致。
func Current(source string) (bool, error) {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return false, err
	}
	reason, err := inspectCurrent(absSource)
	if err != nil {
		return false, err
	}
	return reason == "", nil
}

// inspectCurrent 返回空 reason 表示组件与平台服务完全就绪；
// 返回非空 reason 且 err == nil 表示可通过单次提权安装自动修复；返回 err != nil 表示不可自动修复的安全或输入错误。
func inspectCurrent(source string) (string, error) {
	target, err := ExecutablePath()
	if err != nil {
		return "", err
	}
	pairs := componentPairs(source, target)
	sourceDigests := make([][32]byte, len(pairs))
	for i, pair := range pairs {
		sum, err := digest(pair[0])
		if err != nil {
			return "", err
		}
		sourceDigests[i] = sum
	}
	reason, err := inspectInstalled(target)
	if err != nil {
		return "", err
	}
	if reason != "" {
		return reason, nil
	}
	for i, pair := range pairs {
		installedSum, err := digest(pair[1])
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return fmt.Sprintf("组件待同步 %s", filepath.Base(pair[1])), nil
		}
		if err != nil {
			return "", err
		}
		if sourceDigests[i] != installedSum {
			return fmt.Sprintf("组件版本待更新 %s", filepath.Base(pair[1])), nil
		}
	}
	return checkService()
}

func verifyInstalled(target string) error {
	reason, err := inspectInstalled(target)
	if err != nil {
		return err
	}
	if reason != "" {
		return errors.New(reason)
	}
	return nil
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
