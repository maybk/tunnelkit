package credentials

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tunnelkit/internal/storage"
)

const (
	masterKeySize         = 32
	maxMasterKeyFileBytes = 4 << 10 // 4 KiB
	staleEmptyKeyAge      = 5 * time.Second
)

var (
	// ErrMasterKeyMissing 表示凭据目录中缺少主密钥 secret.key。
	ErrMasterKeyMissing = errors.New("缺少主密钥 secret.key")
	// ErrMasterKeyInvalid 表示主密钥 secret.key 格式无效、为空、非常规文件或已损坏。
	ErrMasterKeyInvalid = errors.New("主密钥 secret.key 无效或已损坏")
	// ErrLegacyKeyMigrationRequired 表示检测到未迁移的旧版 Windows DPAPI 主密钥。
	ErrLegacyKeyMigrationRequired = errors.New("检测到旧版 Windows 专属主密钥，请先在原 Windows 设备升级并启动新版 TunnelKit 完成自动迁移")
	// ErrDecryptFailed 表示主密钥与密文不匹配、密文格式无效或认证失败。
	ErrDecryptFailed = errors.New("解密凭据失败")
)

// dpapiBlobHeader 是 Windows DPAPI CryptProtectData 输出的标准 20 字节版本与提供程序魔数头。
var dpapiBlobHeader = [...]byte{
	0x01, 0x00, 0x00, 0x00,
	0xD0, 0x8C, 0x9D, 0xDF, 0x01, 0x15, 0xD1, 0x11,
	0x8C, 0x7A, 0x00, 0xC0, 0x4F, 0xC2, 0x97, 0xEB,
}

var errKeyFileIncomplete = errors.New("密钥文件正在写入中")

// Sealed 表示 AES-256-GCM 密封后的凭据密文结构。
type Sealed struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// Sealer 使用保存在数据目录 secret.key 的跨平台 256 位主密钥提供并发安全的 AES-256-GCM 加解密，
// 并在 Windows 上自动兼容读取并无感迁移旧版 DPAPI 主密钥。
type Sealer struct {
	mu           sync.Mutex
	keyPath      string
	cachedGCM    cipher.AEAD
	keyInfo      os.FileInfo
	unsealLegacy func([]byte) ([]byte, error)
}

// New 创建指向指定数据根目录的 Sealer。
func New(root string) *Sealer {
	return &Sealer{keyPath: storage.SecretKeyPath(root)}
}

// KeyPath 返回主密钥文件路径。
func (v *Sealer) KeyPath() string {
	return v.keyPath
}

// EnsurePortable 在主密钥文件已存在时完成旧版 Windows DPAPI 主密钥的无感迁移并收紧文件权限；
// 若主密钥文件尚不存在则不做任何操作，绝不提前创建文件。
func (v *Sealer) EnsurePortable() error {
	if _, err := os.Lstat(v.keyPath); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := v.cipher(false); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if storage.HasBroadPermissions(v.keyPath) {
		if err := storage.RestrictPrivateFile(v.keyPath); err != nil {
			return err
		}
	}
	return nil
}

func (v *Sealer) cipher(createIfMissing bool) (cipher.AEAD, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cachedGCM != nil && v.keyInfo != nil {
		if info, err := os.Lstat(v.keyPath); err == nil &&
			info.Mode().IsRegular() &&
			os.SameFile(info, v.keyInfo) &&
			info.ModTime().Equal(v.keyInfo.ModTime()) &&
			info.Size() == v.keyInfo.Size() {
			return v.cachedGCM, nil
		}
		v.cachedGCM = nil
		v.keyInfo = nil
	}

	key, info, legacy, err := v.loadKeyLocked(createIfMissing)
	if err != nil {
		return nil, err
	}
	// 使用完毕后立即归零原始主密钥切片，避免明文主密钥长期驻留内存。
	defer clear(key)

	if legacy {
		info = v.migrateLegacyKeyLocked(key, info)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	v.cachedGCM = gcm
	v.keyInfo = info
	return gcm, nil
}

// Encrypt 使用主密钥将明文密封为 AES-256-GCM 密文；若主密钥文件尚不存在则原子生成新的跨平台主密钥。
func (v *Sealer) Encrypt(plaintext string) (Sealed, error) {
	return v.encrypt(plaintext, true)
}

// EncryptWithExistingKey 仅使用已有主密钥密封明文；若主密钥文件缺失则返回 ErrMasterKeyMissing，
// 用于阻止凭据库在丢失旧密钥时静默生成新密钥导致双密钥混写。
func (v *Sealer) EncryptWithExistingKey(plaintext string) (Sealed, error) {
	return v.encrypt(plaintext, false)
}

func (v *Sealer) encrypt(plaintext string, createIfMissing bool) (Sealed, error) {
	gcm, err := v.cipher(createIfMissing)
	if err != nil {
		return Sealed{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, err
	}
	plain := []byte(plaintext)
	defer clear(plain)
	return Sealed{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	}, nil
}

// Decrypt 使用已有主密钥解封由 Encrypt 生成的密文；只读路径绝不新建或删除主密钥文件。
func (v *Sealer) Decrypt(sealed Sealed) (string, error) {
	gcm, err := v.cipher(false)
	if err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sealed.Nonce))
	if err != nil {
		return "", fmt.Errorf("%w: 解析凭据 nonce 失败: %w", ErrDecryptFailed, err)
	}
	if len(nonce) != gcm.NonceSize() {
		return "", fmt.Errorf("%w: 凭据 nonce 长度无效", ErrDecryptFailed)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sealed.Ciphertext))
	if err != nil {
		return "", fmt.Errorf("%w: 解析凭据密文失败: %w", ErrDecryptFailed, err)
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrDecryptFailed, err)
	}
	defer clear(plain)
	return string(plain), nil
}

func (v *Sealer) loadKeyLocked(createIfMissing bool) ([]byte, os.FileInfo, bool, error) {
	key, info, legacy, err := v.readKey(createIfMissing)
	if err == nil {
		return key, info, legacy, nil
	}
	if errors.Is(err, errKeyFileIncomplete) {
		key, info, legacy, err = v.waitForConcurrentKeyWriteLocked(createIfMissing)
		if err == nil {
			return key, info, legacy, nil
		}
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, false, err
	}
	if !createIfMissing {
		return nil, nil, false, fmt.Errorf("%w: %w", ErrMasterKeyMissing, err)
	}
	return v.createKeyLocked()
}

func (v *Sealer) createKeyLocked() ([]byte, os.FileInfo, bool, error) {
	newKey := make([]byte, masterKeySize)
	if _, err := rand.Read(newKey); err != nil {
		return nil, nil, false, fmt.Errorf("生成主密钥失败: %w", err)
	}
	payload := encodeKeyPayload(newKey)
	defer clear(payload)

	dir := filepath.Dir(v.keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		clear(newKey)
		return nil, nil, false, err
	}

	// 先完整落盘并收紧临时文件权限，再通过 os.Link 原子排他绑定到 secret.key（绝不覆盖已有文件，也无权限暴露窗口）。
	tmp, err := os.CreateTemp(dir, ".secret.key.tmp-*")
	if err != nil {
		clear(newKey)
		return nil, nil, false, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		clear(newKey)
		return nil, nil, false, err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		clear(newKey)
		return nil, nil, false, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		clear(newKey)
		return nil, nil, false, err
	}
	if err := tmp.Close(); err != nil {
		clear(newKey)
		return nil, nil, false, err
	}
	if err := storage.RestrictPrivateFile(tmpPath); err != nil {
		clear(newKey)
		return nil, nil, false, err
	}

	if err := os.Link(tmpPath, v.keyPath); err != nil {
		if errors.Is(err, fs.ErrExist) {
			clear(newKey)
			return v.waitForConcurrentKeyWriteLocked(true)
		}
		// 兼容不支持硬链接的文件系统，回退至排他创建。
		return v.createExclFallbackLocked(newKey, payload)
	}

	info, _ := os.Lstat(v.keyPath)
	return newKey, info, false, nil
}

func (v *Sealer) createExclFallbackLocked(newKey, payload []byte) ([]byte, os.FileInfo, bool, error) {
	f, err := os.OpenFile(v.keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		clear(newKey)
		return v.waitForConcurrentKeyWriteLocked(true)
	}
	if err != nil {
		clear(newKey)
		return nil, nil, false, err
	}
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		_ = os.Remove(v.keyPath)
		clear(newKey)
		return nil, nil, false, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(v.keyPath)
		clear(newKey)
		return nil, nil, false, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(v.keyPath)
		clear(newKey)
		return nil, nil, false, err
	}
	if err := storage.RestrictPrivateFile(v.keyPath); err != nil {
		_ = os.Remove(v.keyPath)
		clear(newKey)
		return nil, nil, false, err
	}
	info, _ := os.Lstat(v.keyPath)
	return newKey, info, false, nil
}

func (v *Sealer) migrateLegacyKeyLocked(key []byte, origInfo os.FileInfo) os.FileInfo {
	if origInfo != nil {
		current, err := os.Lstat(v.keyPath)
		if err != nil {
			return origInfo
		}
		if !current.Mode().IsRegular() ||
			!os.SameFile(current, origInfo) ||
			!current.ModTime().Equal(origInfo.ModTime()) ||
			current.Size() != origInfo.Size() {
			return current
		}
	}

	payload := encodeKeyPayload(key)
	defer clear(payload)
	if err := storage.WritePrivateFile(v.keyPath, payload); err != nil {
		slog.Warn("自动迁移旧版主密钥失败，本次继续使用内存密钥", "error", err)
		return origInfo
	}
	slog.Info("已将旧版 Windows 主密钥自动迁移为跨平台格式")
	if info, err := os.Lstat(v.keyPath); err == nil && info.Mode().IsRegular() {
		return info
	}
	return nil
}

func (v *Sealer) waitForConcurrentKeyWriteLocked(allowStaleCleanup bool) ([]byte, os.FileInfo, bool, error) {
	for range 20 {
		key, info, legacy, readErr := v.readKey(allowStaleCleanup)
		if readErr == nil {
			return key, info, legacy, nil
		}
		// 仅在检测到并发写入中的未完成空文件时等待重试，确定性错误立即返回。
		if !errors.Is(readErr, errKeyFileIncomplete) {
			return nil, nil, false, readErr
		}
		time.Sleep(10 * time.Millisecond)
	}
	key, info, legacy, readErr := v.readKey(allowStaleCleanup)
	if readErr == nil {
		return key, info, legacy, nil
	}
	if errors.Is(readErr, errKeyFileIncomplete) {
		return nil, nil, false, fmt.Errorf("%w: 主密钥文件为空", ErrMasterKeyInvalid)
	}
	return nil, nil, false, readErr
}

func (v *Sealer) readKey(allowStaleCleanup bool) ([]byte, os.FileInfo, bool, error) {
	data, info, err := storage.ReadRegularFileBounded(v.keyPath, maxMasterKeyFileBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, nil, false, err
		}
		if errors.Is(err, os.ErrInvalid) || errors.Is(err, storage.ErrFileTooLarge) {
			return nil, nil, false, fmt.Errorf("%w: %w", ErrMasterKeyInvalid, err)
		}
		return nil, nil, false, err
	}
	defer clear(data)

	trimmed := bytes.TrimSpace(storage.StripUTF8BOM(data))
	if len(trimmed) == 0 {
		if time.Since(info.ModTime()) > staleEmptyKeyAge {
			if allowStaleCleanup {
				if cur, statErr := os.Lstat(v.keyPath); statErr == nil &&
					cur.Mode().IsRegular() &&
					os.SameFile(cur, info) &&
					cur.ModTime().Equal(info.ModTime()) &&
					cur.Size() == info.Size() {
					_ = os.Remove(v.keyPath)
					return nil, nil, false, fs.ErrNotExist
				}
				return nil, nil, false, errKeyFileIncomplete
			}
			return nil, nil, false, fmt.Errorf("%w: 主密钥文件为空", ErrMasterKeyInvalid)
		}
		return nil, nil, false, errKeyFileIncomplete
	}

	raw := make([]byte, base64.StdEncoding.DecodedLen(len(trimmed)))
	n, err := base64.StdEncoding.Decode(raw, trimmed)
	if err != nil {
		clear(raw)
		return nil, nil, false, fmt.Errorf("%w: Base64 解码失败: %w", ErrMasterKeyInvalid, err)
	}
	raw = raw[:n]
	if len(raw) == masterKeySize {
		return raw, info, false, nil
	}
	defer clear(raw)

	if len(raw) > masterKeySize {
		unseal := unsealLegacyKey
		if v.unsealLegacy != nil {
			unseal = v.unsealLegacy
		}
		key, unsealErr := unseal(raw)
		if unsealErr != nil {
			return nil, nil, false, unsealErr
		}
		if len(key) != masterKeySize {
			clear(key)
			return nil, nil, false, fmt.Errorf("%w: 期望 %d 字节主密钥，实际 %d 字节", ErrMasterKeyInvalid, masterKeySize, len(key))
		}
		return key, info, true, nil
	}
	return nil, nil, false, fmt.Errorf("%w: 期望 %d 字节主密钥，实际 %d 字节", ErrMasterKeyInvalid, masterKeySize, len(raw))
}

func encodeKeyPayload(key []byte) []byte {
	payload := make([]byte, base64.StdEncoding.EncodedLen(len(key))+1)
	base64.StdEncoding.Encode(payload, key)
	payload[len(payload)-1] = '\n'
	return payload
}

func hasDPAPIMagic(raw []byte) bool {
	return len(raw) >= len(dpapiBlobHeader) && bytes.Equal(raw[:len(dpapiBlobHeader)], dpapiBlobHeader[:])
}
