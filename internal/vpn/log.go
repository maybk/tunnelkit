package vpn

import (
	"errors"
	"os"
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
	return storage.ResetFile(storage.LogPath(s.rootDir, profileID))
}

// RemoveLog 删除指定配置的连接日志文件（删除配置时调用）。
func (s *Service) RemoveLog(profileID string) error {
	if !profile.IsValidID(profileID) {
		return ErrInvalidProfile
	}
	err := os.Remove(storage.LogPath(s.rootDir, profileID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
