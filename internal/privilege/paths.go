// Package privilege 负责三端受保护辅助组件的安装校验、系统授权与运行期路径管理，
// 不依赖 VPN 引擎、update、GUI 或用户配置存储。
package privilege

import (
	"errors"
	"path/filepath"
)

var ErrCancelled = errors.New("elevation cancelled by user")

const WindowsServiceName = "TunnelKitHelper"

// SessionDir 返回受保护的按用户隔离的辅助进程状态目录。
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

// JournalPath 返回指定用户的网络恢复日志路径（不创建目录）。
func JournalPath(owner string) (string, error) {
	dir, err := SessionDir(owner)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "netcfg-journal.json"), nil
}

// PrepareSession 初始化系统受保护的用户会话目录并返回网络恢复日志路径。
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
