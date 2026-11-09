package profile

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/storage"
)

var ErrNotFound = errors.New("未找到指定的连接配置")

// Store 管理连接配置的增删改查，密码交由 secretVault 加密保存。
type Store struct {
	path    string
	secrets *secretVault
	mu      sync.Mutex
}

// NewStore 创建以 rootDir 为数据目录的配置仓库。
func NewStore(rootDir string) *Store {
	return &Store{
		path:    storage.ProfilesPath(rootDir),
		secrets: newSecretVault(rootDir),
	}
}

// List 返回所有已保存的连接配置。
func (s *Store) List() ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.readAll()
	if err != nil {
		return nil, err
	}
	hasPwd, err := s.secrets.idSet()
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		_, profiles[i].HasPassword = hasPwd[profiles[i].ID]
	}
	return profiles, nil
}

// GetWithPassword 返回配置及其解密后的密码，仅供建立连接时使用。
func (s *Store) GetWithPassword(id string) (Profile, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.readAll()
	if err != nil {
		return Profile{}, "", err
	}
	i := indexOf(profiles, id)
	if i < 0 {
		return Profile{}, "", ErrNotFound
	}
	p := profiles[i]
	if p.Auth == string(fortivpn.AuthSAML) {
		has, err := s.secrets.has(p.ID)
		if err != nil {
			return Profile{}, "", err
		}
		if !has {
			return p, "", nil
		}
	}
	password, err := s.secrets.load(p.ID)
	if err != nil {
		return Profile{}, "", fmt.Errorf("读取“%s”的密码失败: %w", p.Name, err)
	}
	p.HasPassword = true
	return p, password, nil
}

// Save 创建（ID 为空）或更新一个配置；更新时 Password 为空表示保留原密码。
func (s *Store) Save(in Input) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.readAll()
	if err != nil {
		return Profile{}, err
	}

	id := strings.TrimSpace(in.ID)
	existing := indexOf(profiles, id)
	if id != "" && existing < 0 {
		return Profile{}, ErrNotFound
	}

	hasSavedPwd, err := s.secrets.has(id)
	if err != nil {
		return Profile{}, err
	}
	hasSavedPwd = existing >= 0 && hasSavedPwd
	normalized, err := NormalizeAndValidate(in, hasSavedPwd)
	if err != nil {
		return Profile{}, err
	}
	if normalized.ID == "" {
		if normalized.ID, err = newID(); err != nil {
			return Profile{}, fmt.Errorf("生成配置 ID 失败: %w", err)
		}
	}

	if normalized.Password != "" {
		if err := s.secrets.save(normalized.ID, normalized.Password); err != nil {
			return Profile{}, fmt.Errorf("保存密码失败: %w", err)
		}
		hasSavedPwd = true
	}

	item := Profile{
		ID:          normalized.ID,
		Name:        normalized.Name,
		Host:        normalized.Host,
		Port:        normalized.Port,
		Username:    normalized.Username,
		Realm:       normalized.Realm,
		TrustedCert: normalized.TrustedCert,
		Auth:        normalized.Auth,
		Routes:      normalized.Routes,
		ExtraRoutes: normalized.ExtraRoutes,
		DNS:         normalized.DNS,
		DNSDomains:  normalized.DNSDomains,
		Reconnect:   normalized.Reconnect,
		UserAgent:   normalized.UserAgent,
		LegacyTLS:   normalized.LegacyTLS,
		ClientCert:  normalized.ClientCert,
		ClientKey:   normalized.ClientKey,
		HasPassword: hasSavedPwd,
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}
	if existing >= 0 {
		profiles[existing] = item
	} else {
		profiles = append(profiles, item)
	}

	if err := s.writeAll(profiles); err != nil {
		return Profile{}, err
	}
	return item, nil
}

// TrustCert 将网关证书 SHA-256 指纹写入指定配置。
func (s *Store) TrustCert(id, sha256 string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	digest := strings.ToLower(strings.TrimSpace(sha256))
	if !IsValidCertDigest(digest) {
		return Profile{}, errors.New("证书指纹格式无效")
	}

	profiles, err := s.readAll()
	if err != nil {
		return Profile{}, err
	}
	i := indexOf(profiles, id)
	if i < 0 {
		return Profile{}, ErrNotFound
	}
	hasPassword, err := s.secrets.has(id)
	if err != nil {
		return Profile{}, err
	}
	profiles[i].TrustedCert = digest
	profiles[i].UpdatedAt = time.Now().Format(time.RFC3339)
	if err := s.writeAll(profiles); err != nil {
		return Profile{}, err
	}
	p := profiles[i]
	p.HasPassword = hasPassword
	return p, nil
}

// Delete 删除配置并同步清除其密码。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.readAll()
	if err != nil {
		return err
	}
	i := indexOf(profiles, id)
	if i < 0 {
		return ErrNotFound
	}
	if err := s.secrets.remove(id); err != nil {
		return err
	}
	return s.writeAll(append(profiles[:i], profiles[i+1:]...))
}

func indexOf(profiles []Profile, id string) int {
	if id == "" {
		return -1
	}
	for i := range profiles {
		if profiles[i].ID == id {
			return i
		}
	}
	return -1
}
