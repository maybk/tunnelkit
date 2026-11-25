//go:build !windows

package storage

import "os"

// RestrictPrivateFile 以不跟随符号链接的句柄将类 Unix 常规文件权限收紧至 0600。
func RestrictPrivateFile(path string) error {
	f, err := OpenRegularNoFollow(path)
	if err != nil {
		return err
	}
	chmodErr := f.Chmod(0o600)
	closeErr := f.Close()
	if chmodErr != nil {
		return chmodErr
	}
	return closeErr
}

// HasBroadPermissions 检查类 Unix 常规私有文件是否向当前用户以外的主体暴露了读写执行权限。
func HasBroadPermissions(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false
	}
	return (info.Mode().Perm() & 0o077) != 0
}

func clearReadOnlyIfRegular(string) {}
