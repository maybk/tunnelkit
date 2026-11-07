package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
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

// Sealed 表示 AES-256-GCM 密封后的凭据密文结构。
type Sealed struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// Sealer 使用保存在本机 secret.key 的主密钥（Windows 下额外通过 DPAPI 保护）
// 提供 AES-256-GCM 加解密能力。
type Sealer struct {
	keyPath   string
	cachedKey []byte
}

var keyMu sync.Mutex

// New 创建指向指定数据根目录的 Sealer。
func New(root string) *Sealer {
	return &Sealer{keyPath: storage.SecretKeyPath(root)}
}

func (v *Sealer) cipher() (cipher.AEAD, error) {
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

// Encrypt 使用本机主密钥将明文密封为 AES-256-GCM 密文。
func (v *Sealer) Encrypt(plaintext string) (Sealed, error) {
	gcm, err := v.cipher()
	if err != nil {
		return Sealed{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, err
	}
	return Sealed{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, []byte(plaintext), nil)),
	}, nil
}

// Decrypt 解封由 Encrypt 生成的密文。
func (v *Sealer) Decrypt(sealed Sealed) (string, error) {
	gcm, err := v.cipher()
	if err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(sealed.Nonce)
	if err != nil {
		return "", fmt.Errorf("解析凭据 nonce 失败: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return "", errors.New("凭据 nonce 长度无效")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(sealed.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("解析凭据密文失败: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密凭据失败: %w", err)
	}
	return string(plain), nil
}

func (v *Sealer) loadOrCreateKey() ([]byte, error) {
	keyMu.Lock()
	defer keyMu.Unlock()
	if len(v.cachedKey) == 32 {
		return v.cachedKey, nil
	}
	if key, err := v.readKey(); err == nil {
		v.cachedKey = key
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return nil, fmt.Errorf("生成加密主密钥失败: %w", err)
	}
	sealedKey, err := sealKeyBytes(newKey)
	if err != nil {
		return nil, fmt.Errorf("保护加密主密钥失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(v.keyPath), 0o700); err != nil {
		return nil, err
	}
	// O_EXCL 防止并发首次创建时后写者覆盖密钥，导致已有密文无法解密。
	f, err := os.OpenFile(v.keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		for range 10 {
			if key, readErr := v.readKey(); readErr == nil {
				v.cachedKey = key
				return key, nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		key, readErr := v.readKey()
		if readErr == nil {
			v.cachedKey = key
		}
		return key, readErr
	}
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(sealedKey) + "\n"); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	v.cachedKey = newKey
	return newKey, nil
}

func (v *Sealer) readKey() ([]byte, error) {
	data, err := os.ReadFile(v.keyPath)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("解析加密主密钥失败: %w", err)
	}
	key, err := unsealKeyBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("解封加密主密钥失败: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("加密主密钥长度不正确")
	}
	return key, nil
}
