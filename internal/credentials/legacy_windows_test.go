//go:build windows

package credentials

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"tunnelkit/internal/storage"
)

func protectWithWindowsDPAPIForTest(t *testing.T, raw []byte) []byte {
	t.Helper()
	inBlob := windows.DataBlob{
		Size: uint32(len(raw)),
		Data: &raw[0],
	}
	var outBlob windows.DataBlob
	if err := windows.CryptProtectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &outBlob); err != nil {
		t.Fatalf("CryptProtectData: %v", err)
	}
	defer func() {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
	}()
	out := make([]byte, outBlob.Size)
	copy(out, unsafe.Slice(outBlob.Data, outBlob.Size))
	return out
}

func TestWindowsLegacyDPAPIKeySeamlessMigration(t *testing.T) {
	rawKey := make([]byte, masterKeySize)
	if _, err := rand.Read(rawKey); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	// 使用同一 32 字节原始密钥生成历史密文，模拟旧版 Windows 客户端保存的密码。
	seedRoot := t.TempDir()
	if err := storage.WritePrivateFile(storage.SecretKeyPath(seedRoot), encodeKeyPayload(rawKey)); err != nil {
		t.Fatalf("write seed portable key: %v", err)
	}
	legacyCiphertext, err := New(seedRoot).Encrypt("win-dpapi-saved-password")
	if err != nil {
		t.Fatalf("seed Encrypt: %v", err)
	}

	// 在新目录写入真实 Win32 DPAPI 封装的旧版 secret.key。
	root := t.TempDir()
	keyPath := storage.SecretKeyPath(root)
	dpapiBlob := protectWithWindowsDPAPIForTest(t, rawKey)
	if !hasDPAPIMagic(dpapiBlob) {
		t.Fatal("expected Win32 CryptProtectData output to start with standard DPAPI header")
	}
	if err := storage.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(dpapiBlob)+"\n"), 0o600); err != nil {
		t.Fatalf("write legacy DPAPI key: %v", err)
	}

	// EnsurePortable 应无感完成解封、转换为跨平台 32 字节 Base64 并收紧 Protected DACL。
	if err := New(root).EnsurePortable(); err != nil {
		t.Fatalf("EnsurePortable failed on real DPAPI key: %v", err)
	}
	migratedRaw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile migrated key: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(migratedRaw)))
	if err != nil || !bytes.Equal(decoded, rawKey) {
		t.Fatalf("migrated key mismatch: len=%d err=%v", len(decoded), err)
	}
	if storage.HasBroadPermissions(keyPath) {
		t.Fatal("expected migrated secret.key to have tightened Windows Protected DACL")
	}

	// 历史密文无需重新加密即可直接解封。
	got, err := New(root).Decrypt(legacyCiphertext)
	if err != nil || got != "win-dpapi-saved-password" {
		t.Fatalf("Decrypt after DPAPI migration = (%q, %v)", got, err)
	}
}
