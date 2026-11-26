package profile

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"tunnelkit/internal/storage"
)

const (
	settingsSchemaVersion   = 1
	maxSettingsDocumentSize = 256 << 10 // 256 KiB
)

// AppSettings 表示桌面客户端应用级偏好设置。
type AppSettings struct {
	Theme           string `json:"theme"`
	AutoCheckUpdate bool   `json:"autoCheckUpdate"`
}

// SettingsPatch 只包含调用方明确修改的应用设置字段。
type SettingsPatch struct {
	Theme           *string `json:"theme,omitempty"`
	AutoCheckUpdate *bool   `json:"autoCheckUpdate,omitempty"`
}

// DefaultSettings 返回默认应用设置。
func DefaultSettings() AppSettings {
	return AppSettings{
		Theme:           "system",
		AutoCheckUpdate: true,
	}
}

// Apply 将显式提供的字段合并到当前设置快照，未提供字段保持不变。
func (p SettingsPatch) Apply(current AppSettings) AppSettings {
	if p.Theme != nil {
		current.Theme = *p.Theme
	}
	if p.AutoCheckUpdate != nil {
		current.AutoCheckUpdate = *p.AutoCheckUpdate
	}
	return current
}

// GetSettings 读取应用通用设置；文件不存在或为空时返回默认值。
func (s *Store) GetSettings() (AppSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readSettingsLocked()
}

// PatchSettings 在同一把锁内读取、合并、归一化并原子落盘应用设置。
func (s *Store) PatchSettings(patch SettingsPatch) (AppSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.readSettingsLocked()
	if err != nil {
		return AppSettings{}, err
	}
	saved := normalizeAppSettings(patch.Apply(current))
	if err := s.writeSettingsLocked(saved); err != nil {
		return AppSettings{}, err
	}
	return saved, nil
}

func (s *Store) readSettingsLocked() (AppSettings, error) {
	def := DefaultSettings()
	data, _, err := storage.ReadRegularFileBounded(s.settingsPath, maxSettingsDocumentSize)
	if errors.Is(err, fs.ErrNotExist) {
		return def, nil
	}
	if err != nil {
		return def, fmt.Errorf("读取应用设置文件失败: %w", err)
	}
	trimmed := bytes.TrimSpace(storage.StripUTF8BOM(data))
	if len(trimmed) == 0 {
		return def, nil
	}
	payload, _, err := storage.DecodeDocument(trimmed, settingsSchemaVersion)
	if err != nil {
		return def, fmt.Errorf("解析应用设置文件失败: %w", err)
	}
	out := def
	if err := json.Unmarshal(payload, &out); err != nil {
		return def, fmt.Errorf("解析应用设置文件失败: %w", err)
	}
	return normalizeAppSettings(out), nil
}

func (s *Store) writeSettingsLocked(settings AppSettings) error {
	data, err := storage.MarshalDocument(settingsSchemaVersion, settings)
	if err != nil {
		return err
	}
	return storage.WritePrivateFile(s.settingsPath, append(data, '\n'))
}

func normalizeAppSettings(settings AppSettings) AppSettings {
	settings.Theme = normalizeThemeMode(settings.Theme)
	return settings
}

func normalizeThemeMode(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch trimmed {
	case "light", "dark", "system":
		return trimmed
	default:
		return "system"
	}
}
