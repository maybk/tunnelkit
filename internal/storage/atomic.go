package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// WriteFile 在目标目录内通过临时文件 + Rename 原子写入文件，并强制应用指定权限。
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return WriteFileWithHook(path, data, perm, nil)
}

// WritePrivateFile 以仅当前用户可读写权限原子写入敏感或私有文件：
// 在临时文件落盘后、原子替换目标前收紧系统文件权限（Unix 0600，Windows NTFS 受保护 DACL）。
func WritePrivateFile(path string, data []byte) error {
	return WriteFileWithHook(path, data, 0o600, RestrictPrivateFile)
}

// WriteFileWithHook 在目标目录内通过临时文件 + Rename 原子写入文件；
// 若提供 beforeRename，则在临时文件关闭落盘后、原子重命名之前执行，失败时清理临时文件且不覆盖原目标。
func WriteFileWithHook(path string, data []byte, perm os.FileMode, beforeRename func(tmpPath string) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if beforeRename != nil {
		if err := beforeRename(tmpPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if err := renameWithRetry(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func renameWithRetry(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	clearReadOnlyIfRegular(dst)
	for range 3 {
		time.Sleep(5 * time.Millisecond)
		if err = os.Rename(src, dst); err == nil {
			return nil
		}
	}
	return err
}
