// Package storage 负责应用本地数据目录布局、日志文件读取与原子文件写入。
package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultDirName = ".tunnelkit"

// DefaultRoot 返回默认数据根目录（~/.tunnelkit）。
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	return filepath.Join(home, defaultDirName), nil
}

// ProfilesPath 返回服务配置列表文件路径。
func ProfilesPath(rootDir string) string {
	return filepath.Join(rootDir, "profiles.json")
}

// SecretsPath 返回加密密码存储文件路径。
func SecretsPath(rootDir string) string {
	return filepath.Join(rootDir, "secrets.json")
}

// SecretKeyPath 返回本机随机密钥文件路径。
func SecretKeyPath(rootDir string) string {
	return filepath.Join(rootDir, "secret.key")
}

// LogsDir 返回连接日志目录路径。
func LogsDir(rootDir string) string {
	return filepath.Join(rootDir, "logs")
}

// LogPath 返回指定配置的连接日志文件路径。
func LogPath(rootDir, profileID string) string {
	return filepath.Join(LogsDir(rootDir), profileID+".log")
}

// RunDir 返回运行时状态目录路径。
func RunDir(rootDir string) string {
	return filepath.Join(rootDir, "run")
}

// SessionPath 返回当前连接会话凭据文件路径。
func SessionPath(rootDir string) string {
	return filepath.Join(RunDir(rootDir), "session.json")
}

// TicketPath 是 SessionPath 的别名，用于保存 helper IPC 会话凭据。
func TicketPath(rootDir string) string {
	return SessionPath(rootDir)
}

// HelperSocketPath 返回 macOS / Linux 下 helper 的 Unix domain socket 路径。
func HelperSocketPath(rootDir string) string {
	return filepath.Join(RunDir(rootDir), "helper.sock")
}

// JournalPath 返回网络配置回滚日志文件路径。
func JournalPath(rootDir string) string {
	return filepath.Join(RunDir(rootDir), "netcfg-journal.json")
}
