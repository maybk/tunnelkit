package vpn

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tunnelkit/internal/profile"
	"tunnelkit/internal/storage"
)

// Log 读取指定配置的脱敏连接日志。
func (s *Service) Log(profileID string) (LogView, error) {
	if !profile.IsValidID(profileID) {
		return LogView{}, ErrInvalidProfile
	}
	tail, err := storage.TailFile(storage.LogPath(s.rootDir, profileID), storage.DefaultTailBytes)
	if err != nil {
		return LogView{}, err
	}
	view := LogView{ProfileID: profileID, Content: tail.Content, Exists: tail.Exists}
	if tail.Exists {
		view.UpdatedAt = tail.ModTime.Format(time.RFC3339)
	}
	return view, nil
}

// ClearLog 清空指定配置的连接日志。
func (s *Service) ClearLog(profileID string) error {
	if !profile.IsValidID(profileID) {
		return ErrInvalidProfile
	}
	return s.resetLog(profileID)
}

// RemoveLog 删除指定配置的连接日志文件（删除配置时调用）。
func (s *Service) RemoveLog(profileID string) error {
	if !profile.IsValidID(profileID) {
		return ErrInvalidProfile
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	err := os.Remove(storage.LogPath(s.rootDir, profileID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Service) resetLog(profileID string) error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return storage.ResetFile(storage.LogPath(s.rootDir, profileID))
}

var logLineSanitizer = strings.NewReplacer("\r", " ", "\n", " ")

func (s *Service) appendLogf(profileID, level, format string, args ...any) {
	if !profile.IsValidID(profileID) {
		return
	}
	msg := logLineSanitizer.Replace(strings.TrimSpace(fmt.Sprintf(format, args...)))
	if msg == "" {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := storage.Redact(fmt.Sprintf("%s %-5s %s\n", ts, level, msg))

	s.logMu.Lock()
	defer s.logMu.Unlock()

	path := storage.LogPath(s.rootDir, profileID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		slog.Warn("创建连接日志目录失败", "error", err)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Warn("打开连接日志失败", "error", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		slog.Warn("写入连接日志失败", "error", err)
	}
}

func (s *Service) appendErrorLog(profileID string, vpnErr *Error, cause error) {
	if vpnErr == nil && cause == nil {
		return
	}
	var parts []string
	if vpnErr != nil && vpnErr.Message != "" {
		parts = append(parts, vpnErr.Message)
	}
	if cause != nil {
		detail := strings.TrimSpace(cause.Error())
		if detail != "" && (len(parts) == 0 || !strings.Contains(parts[0], detail)) {
			parts = append(parts, detail)
		}
	} else if vpnErr != nil && vpnErr.Hint != "" {
		parts = append(parts, vpnErr.Hint)
	}
	if len(parts) == 0 {
		return
	}
	s.appendLogf(profileID, "ERROR", "%s", strings.Join(parts, ": "))
}
