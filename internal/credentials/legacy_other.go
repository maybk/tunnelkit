//go:build !windows

package credentials

import "fmt"

// unsealLegacyKey 在非 Windows 平台上识别未迁移的旧版 Windows DPAPI 主密钥，并返回明确迁移指引。
func unsealLegacyKey(raw []byte) ([]byte, error) {
	if hasDPAPIMagic(raw) {
		return nil, fmt.Errorf("%w: %w", ErrMasterKeyInvalid, ErrLegacyKeyMigrationRequired)
	}
	return nil, fmt.Errorf("%w: 期望 %d 字节主密钥，实际 %d 字节", ErrMasterKeyInvalid, masterKeySize, len(raw))
}
