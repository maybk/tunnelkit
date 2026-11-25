//go:build windows

package credentials

import (
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

// unsealLegacyKey 在 Windows 上兼容解封旧版通过当前用户 DPAPI 封装的主密钥。
func unsealLegacyKey(sealed []byte) ([]byte, error) {
	if len(sealed) == 0 || len(sealed) > math.MaxUint32 {
		return nil, fmt.Errorf("%w: 旧版主密钥体积超出限制", ErrMasterKeyInvalid)
	}
	inBlob := windows.DataBlob{
		Size: uint32(len(sealed)), //nolint:gosec // G115: 上方已校验 len(sealed) <= math.MaxUint32
		Data: &sealed[0],
	}
	var outBlob windows.DataBlob
	if err := windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &outBlob); err != nil {
		if hasDPAPIMagic(sealed) {
			return nil, fmt.Errorf("%w: %w", ErrMasterKeyInvalid, ErrLegacyKeyMigrationRequired)
		}
		return nil, fmt.Errorf("%w: 解封旧版主密钥失败: %w", ErrMasterKeyInvalid, err)
	}
	defer func() {
		if outBlob.Data != nil && outBlob.Size > 0 {
			clear(unsafe.Slice(outBlob.Data, outBlob.Size)) //nolint:gosec // G103: 归零 Win32 DPAPI 解封后的明文缓冲区
		}
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data))) //nolint:gosec // G103: 释放 Win32 DPAPI 输出缓冲区
	}()

	out := make([]byte, outBlob.Size)
	if outBlob.Size > 0 {
		copy(out, unsafe.Slice(outBlob.Data, outBlob.Size)) //nolint:gosec // G103: 复制 Win32 DPAPI 输出缓冲区
	}
	return out, nil
}
