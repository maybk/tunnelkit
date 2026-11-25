package profile

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"

	"tunnelkit/internal/credentials"
	"tunnelkit/internal/storage"
)

var errSecretNotFound = errors.New("未找到已保存的密码")

type secretAccessError struct {
	msg string
	err error
}

func (e *secretAccessError) Error() string { return e.msg }
func (e *secretAccessError) Unwrap() error { return e.err }

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
		return wrapSecretError("保存密码失败", err, false)
	}
	// 库中若已存在除当前配置外的其他密码条目，必须复用已有主密钥，
	// 严禁在缺少旧 secret.key 时自动新建主密钥造成双密钥混写损坏。
	hasOther := hasOtherSecretKey(secrets, profileID)
	var sealed credentials.Sealed
	if hasOther {
		sealed, err = v.sealer.EncryptWithExistingKey(password)
	} else {
		sealed, err = v.sealer.Encrypt(password)
	}
	if err != nil {
		return wrapSecretError("保存密码失败", err, hasOther)
	}
	secrets[profileID] = sealed
	if err := v.writeMap(secrets); err != nil {
		return wrapSecretError("保存密码失败", err, false)
	}
	return nil
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

func (v *secretVault) has(profileID string) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	secrets, err := v.readMap()
	if err != nil {
		return false, err
	}
	_, ok := secrets[profileID]
	return ok, nil
}

func (v *secretVault) idSet() (map[string]struct{}, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	secrets, err := v.readMap()
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(secrets))
	for id := range secrets {
		set[id] = struct{}{}
	}
	return set, nil
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

func (v *secretVault) prepareStorage() {
	v.mu.Lock()
	defer v.mu.Unlock()

	if err := v.sealer.EnsurePortable(); err != nil {
		slog.Warn("检查或迁移主密钥失败", "error", err)
	}
	tightenPrivateFileIfBroad(v.secretsPath, "密码文件")
}

func (v *secretVault) readMap() (map[string]credentials.Sealed, error) {
	data, _, err := storage.ReadRegularFileBounded(v.secretsPath, maxProfileDocumentSize)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]credentials.Sealed{}, nil
	}
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(storage.StripUTF8BOM(data))
	if len(trimmed) == 0 {
		return map[string]credentials.Sealed{}, nil
	}
	payload, _, err := storage.DecodeDocument(trimmed, secretSchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("解析密码文件失败: %w", err)
	}
	var m map[string]credentials.Sealed
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("解析密码文件失败: %w", err)
	}
	if m == nil {
		m = map[string]credentials.Sealed{}
	}
	return m, nil
}

func (v *secretVault) writeMap(m map[string]credentials.Sealed) error {
	data, err := storage.MarshalDocument(secretSchemaVersion, m)
	if err != nil {
		return err
	}
	return storage.WritePrivateFile(v.secretsPath, append(data, '\n'))
}

func hasOtherSecretKey(existing map[string]credentials.Sealed, target string) bool {
	if len(existing) == 0 {
		return false
	}
	if len(existing) > 1 {
		return true
	}
	_, exists := existing[target]
	return !exists
}

func wrapSecretError(prefix string, err error, guardMixedKey bool) error {
	switch {
	case errors.Is(err, credentials.ErrMasterKeyMissing):
		if guardMixedKey {
			return &secretAccessError{
				msg: prefix + "：检测到已有加密密码但缺少主密钥 secret.key，已阻止写入以防损坏原有密码；请恢复原 secret.key，或删除 secrets.json 后重新保存",
				err: err,
			}
		}
		return &secretAccessError{
			msg: prefix + "：缺少主密钥 secret.key；跨设备复制时请将 profiles.json、secrets.json 与 secret.key 一并复制",
			err: err,
		}
	case errors.Is(err, credentials.ErrLegacyKeyMigrationRequired):
		return &secretAccessError{
			msg: prefix + "：检测到旧版 Windows 专属主密钥；请先在原 Windows 设备升级并启动新版 TunnelKit 完成自动迁移，再复制 secrets.json 与 secret.key",
			err: err,
		}
	case errors.Is(err, credentials.ErrMasterKeyInvalid):
		return &secretAccessError{
			msg: prefix + "：主密钥 secret.key 无效或已损坏；请从原设备重新复制，或删除损坏的 secret.key 与 secrets.json 后重新保存密码",
			err: err,
		}
	case errors.Is(err, credentials.ErrDecryptFailed):
		return &secretAccessError{
			msg: prefix + "：密码解密失败；请确认 secrets.json 与 secret.key 来自同一设备且未被修改，或在编辑连接时重新输入密码",
			err: err,
		}
	case errors.Is(err, fs.ErrPermission):
		return &secretAccessError{
			msg: prefix + "：无权访问本地凭据文件；请检查 ~/.tunnelkit 目录下 secret.key 与 secrets.json 的读写权限",
			err: err,
		}
	default:
		return fmt.Errorf("%s: %w", prefix, err)
	}
}

func tightenPrivateFileIfBroad(path, label string) {
	if !storage.HasBroadPermissions(path) {
		return
	}
	if err := storage.RestrictPrivateFile(path); err != nil {
		slog.Warn("收紧本地文件访问权限失败", "target", label, "error", err)
	}
}
