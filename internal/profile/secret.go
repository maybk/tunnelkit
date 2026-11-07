package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"

	"tunnelkit/internal/credentials"
	"tunnelkit/internal/storage"
)

var errSecretNotFound = errors.New("未找到已保存的密码")

// secretVault 使用 credentials.Sealer 将密码加密保存在 secrets.json。
type secretVault struct {
	secretsPath string
	sealer      *credentials.Sealer
	mu          sync.Mutex
}

func newSecretVault(rootDir string) *secretVault {
	return &secretVault{
		secretsPath: storage.SecretsPath(rootDir),
		sealer:      credentials.New(rootDir),
	}
}

func (v *secretVault) save(profileID, password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	secrets, err := v.readMap()
	if err != nil {
		return err
	}
	sealed, err := v.sealer.Encrypt(password)
	if err != nil {
		return err
	}
	secrets[profileID] = sealed
	return v.writeMap(secrets)
}

func (v *secretVault) load(profileID string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	secrets, err := v.readMap()
	if err != nil {
		return "", err
	}
	sealed, ok := secrets[profileID]
	if !ok {
		return "", errSecretNotFound
	}
	return v.sealer.Decrypt(sealed)
}

func (v *secretVault) has(profileID string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	secrets, err := v.readMap()
	if err != nil {
		return false
	}
	_, ok := secrets[profileID]
	return ok
}

func (v *secretVault) idSet() map[string]struct{} {
	v.mu.Lock()
	defer v.mu.Unlock()

	secrets, err := v.readMap()
	if err != nil || len(secrets) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(secrets))
	for id := range secrets {
		set[id] = struct{}{}
	}
	return set
}

func (v *secretVault) remove(profileID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	secrets, err := v.readMap()
	if err != nil {
		return err
	}
	if _, ok := secrets[profileID]; !ok {
		return nil
	}
	delete(secrets, profileID)
	return v.writeMap(secrets)
}

func (v *secretVault) readMap() (map[string]credentials.Sealed, error) {
	data, err := os.ReadFile(v.secretsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]credentials.Sealed{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]credentials.Sealed
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析密码文件失败: %w", err)
	}
	if m == nil {
		m = map[string]credentials.Sealed{}
	}
	return m, nil
}

func (v *secretVault) writeMap(m map[string]credentials.Sealed) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(v.secretsPath, append(data, '\n'), 0o600)
}
