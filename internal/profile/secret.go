package profile

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tunnelkit/internal/storage"
)

var errSecretNotFound = errors.New("未找到已保存的密码")

type sealedSecret struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// secretVault 使用 AES-256-GCM 将密码加密保存在 secrets.json，密钥保存在本机 secret.key
// （Windows 下密钥额外通过当前用户 DPAPI 保护）。
type secretVault struct {
	secretsPath string
	keyPath     string
	mu          sync.Mutex
}

func newSecretVault(rootDir string) *secretVault {
	return &secretVault{
		secretsPath: storage.SecretsPath(rootDir),
		keyPath:     storage.SecretKeyPath(rootDir),
	}
}

func (v *secretVault) save(profileID, password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	secrets, err := v.readMap()
	if err != nil {
		return err
	}
	sealed, err := v.encrypt(password)
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
	return v.decrypt(sealed)
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

func (v *secretVault) cipher() (cipher.AEAD, error) {
	key, err := v.loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (v *secretVault) encrypt(plaintext string) (sealedSecret, error) {
	gcm, err := v.cipher()
	if err != nil {
		return sealedSecret{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return sealedSecret{}, err
	}
	return sealedSecret{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, []byte(plaintext), nil)),
	}, nil
}

func (v *secretVault) decrypt(sealed sealedSecret) (string, error) {
	gcm, err := v.cipher()
	if err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(sealed.Nonce)
	if err != nil {
		return "", fmt.Errorf("解析密码 nonce 失败: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(sealed.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("解析密码密文失败: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密密码失败: %w", err)
	}
	return string(plain), nil
}

func (v *secretVault) loadOrCreateKey() ([]byte, error) {
	if key, err := v.readKey(); err == nil {
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return nil, fmt.Errorf("生成加密密钥失败: %w", err)
	}
	sealedKey, err := sealKeyBytes(newKey)
	if err != nil {
		return nil, fmt.Errorf("保护加密密钥失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(v.keyPath), 0o700); err != nil {
		return nil, err
	}
	// O_EXCL 防止并发首次创建时后写者覆盖密钥，导致已有密文无法解密。
	f, err := os.OpenFile(v.keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		for range 10 {
			if key, readErr := v.readKey(); readErr == nil {
				return key, nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return v.readKey()
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(sealedKey) + "\n"); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return newKey, nil
}

func (v *secretVault) readKey() ([]byte, error) {
	data, err := os.ReadFile(v.keyPath)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("解析加密密钥失败: %w", err)
	}
	key, err := unsealKeyBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("解封加密密钥失败: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("加密密钥长度不正确")
	}
	return key, nil
}

func (v *secretVault) readMap() (map[string]sealedSecret, error) {
	data, err := os.ReadFile(v.secretsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]sealedSecret{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]sealedSecret
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析密码文件失败: %w", err)
	}
	if m == nil {
		m = map[string]sealedSecret{}
	}
	return m, nil
}

func (v *secretVault) writeMap(m map[string]sealedSecret) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(v.secretsPath, append(data, '\n'), 0o600)
}
