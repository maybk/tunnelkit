package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/storage"
)

const profileSchemaVersion = 1
const secretSchemaVersion = 1

// decodeProfiles 兼容已发布的无版本列表，迁移不触碰密码与密钥。
func decodeProfiles(data []byte, out *[]Profile) error {
	payload, _, err := storage.DecodeDocument(data, profileSchemaVersion)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func (s *Store) readAll() ([]Profile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []Profile{}, nil
	}

	var list []Profile
	if err := decodeProfiles(trimmed, &list); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	valid := list[:0]
	for _, p := range list {
		if !IsValidID(p.ID) {
			slog.Warn("忽略 ID 非法的配置条目", "id", p.ID)
			continue
		}
		if p.Port == 0 {
			p.Port = DefaultPort
		}
		if p.Auth == "" {
			p.Auth = string(fortivpn.AuthPassword)
		}
		if p.Routes == "" {
			p.Routes = string(fortivpn.RouteGateway)
		}
		if p.DNS == "" {
			p.DNS = string(fortivpn.DNSSplit)
		}
		valid = append(valid, p)
	}
	return valid, nil
}

func (s *Store) writeAll(profiles []Profile) error {
	// HasPassword 由 secretVault 实时计算，不落盘。
	persisted := make([]Profile, len(profiles))
	for i, p := range profiles {
		p.HasPassword = false
		persisted[i] = p
	}
	data, err := storage.MarshalDocument(profileSchemaVersion, persisted)
	if err != nil {
		return err
	}
	return storage.WriteFile(s.path, append(data, '\n'), 0o600)
}
