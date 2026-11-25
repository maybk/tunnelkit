package credentials

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tunnelkit/internal/storage"
)

func TestSealerRoundTripAndPortableKeyPersistence(t *testing.T) {
	root := t.TempDir()
	s1 := New(root)
	sealed, err := s1.Encrypt("top-secret-value")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if sealed.Nonce == "" || sealed.Ciphertext == "" {
		t.Fatalf("expected non-empty sealed payload: %+v", sealed)
	}

	// 验证落盘主密钥为三端通用的标准 Base64 32 字节原始密钥。
	rawKeyFile, err := os.ReadFile(storage.SecretKeyPath(root))
	if err != nil {
		t.Fatalf("ReadFile secret.key: %v", err)
	}
	decodedKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawKeyFile)))
	if err != nil {
		t.Fatalf("DecodeString secret.key: %v", err)
	}
	if len(decodedKey) != masterKeySize {
		t.Fatalf("decoded secret.key len = %d, want %d", len(decodedKey), masterKeySize)
	}
	if storage.HasBroadPermissions(storage.SecretKeyPath(root)) {
		t.Fatal("expected newly created secret.key not to have broad permissions")
	}

	// 新建实例从磁盘加载同一密钥解封。
	s2 := New(root)
	plain, err := s2.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if plain != "top-secret-value" {
		t.Fatalf("Decrypt = %q, want %q", plain, "top-secret-value")
	}

	// 模拟跨端编辑带入 UTF-8 BOM 与 CRLF，验证依然正常解密。
	bomCRLF := append([]byte{0xEF, 0xBB, 0xBF}, []byte(strings.TrimSpace(string(rawKeyFile))+"\r\n")...)
	if err := storage.WritePrivateFile(storage.SecretKeyPath(root), bomCRLF); err != nil {
		t.Fatalf("WritePrivateFile BOM/CRLF: %v", err)
	}
	plainBOM, err := New(root).Decrypt(sealed)
	if err != nil || plainBOM != "top-secret-value" {
		t.Fatalf("Decrypt with BOM/CRLF = (%q, %v)", plainBOM, err)
	}
}

func TestSealerRejectsInvalidNonceAndCiphertext(t *testing.T) {
	s := New(t.TempDir())
	sealed, err := s.Encrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Decrypt(Sealed{Nonce: "bad-base64!", Ciphertext: "AA=="}); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("expected ErrDecryptFailed for invalid base64 nonce, got %v", err)
	}
	if _, err := s.Decrypt(Sealed{Nonce: "AA==", Ciphertext: "AA=="}); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("expected ErrDecryptFailed for invalid nonce length, got %v", err)
	}
	sealed.Ciphertext = "AAAA"
	if _, err := s.Decrypt(sealed); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("expected ErrDecryptFailed for tampered ciphertext, got %v", err)
	}
}

func TestSealerDecryptAndExistingKeyNeverCreateOrDeleteKey(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		root := t.TempDir()
		s := New(root)

		if _, err := s.Decrypt(Sealed{Nonce: "AAAAAAAAAAAAAAAA", Ciphertext: "AAAA"}); !errors.Is(err, ErrMasterKeyMissing) {
			t.Fatalf("Decrypt missing key err = %v, want ErrMasterKeyMissing", err)
		}
		if _, err := s.EncryptWithExistingKey("secret"); !errors.Is(err, ErrMasterKeyMissing) {
			t.Fatalf("EncryptWithExistingKey missing key err = %v, want ErrMasterKeyMissing", err)
		}
		if _, err := os.Stat(storage.SecretKeyPath(root)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("expected secret.key not to be created, stat err=%v", err)
		}
	})

	t.Run("stale empty key untouched on read and cleaned on write", func(t *testing.T) {
		root := t.TempDir()
		keyPath := storage.SecretKeyPath(root)
		if err := storage.WritePrivateFile(keyPath, nil); err != nil {
			t.Fatalf("WritePrivateFile empty: %v", err)
		}
		oldTime := time.Now().Add(-10 * time.Second)
		if err := os.Chtimes(keyPath, oldTime, oldTime); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}

		s := New(root)
		if _, err := s.Decrypt(Sealed{Nonce: "AAAAAAAAAAAAAAAA", Ciphertext: "AAAA"}); !errors.Is(err, ErrMasterKeyInvalid) {
			t.Fatalf("Decrypt stale empty key err = %v, want ErrMasterKeyInvalid", err)
		}
		if _, err := os.Stat(keyPath); err != nil {
			t.Fatalf("Decrypt must not delete stale empty key file: %v", err)
		}

		sealed, err := s.Encrypt("recovered-after-stale-empty")
		if err != nil {
			t.Fatalf("Encrypt after stale empty key failed: %v", err)
		}
		got, err := New(root).Decrypt(sealed)
		if err != nil || got != "recovered-after-stale-empty" {
			t.Fatalf("Decrypt after recovery = (%q, %v)", got, err)
		}
	})
}

func TestSealerSeamlessLegacyKeyMigration(t *testing.T) {
	realKey := make([]byte, masterKeySize)
	if _, err := rand.Read(realKey); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	legacyBlob := append(append([]byte(nil), dpapiBlobHeader[:]...), realKey...)
	legacyEncoded := base64.StdEncoding.EncodeToString(legacyBlob) + "\n"

	// 先使用相同 32 字节原始密钥在另一目录生成历史密文，模拟升级前已保存的 secrets.json 密文。
	seedRoot := t.TempDir()
	if err := storage.WritePrivateFile(storage.SecretKeyPath(seedRoot), encodeKeyPayload(realKey)); err != nil {
		t.Fatalf("write seed key: %v", err)
	}
	oldSealed, err := New(seedRoot).Encrypt("legacy-vpn-password")
	if err != nil {
		t.Fatalf("seed Encrypt: %v", err)
	}

	fakeUnseal := func(in []byte) ([]byte, error) {
		if !bytes.Equal(in, legacyBlob) {
			return nil, ErrMasterKeyInvalid
		}
		out := make([]byte, len(realKey))
		copy(out, realKey)
		return out, nil
	}

	t.Run("automatic atomic migration on read", func(t *testing.T) {
		root := t.TempDir()
		keyPath := storage.SecretKeyPath(root)
		if err := storage.WritePrivateFile(keyPath, []byte(legacyEncoded)); err != nil {
			t.Fatalf("write legacy key: %v", err)
		}

		var calls atomic.Int32
		s := New(root)
		s.unsealLegacy = func(in []byte) ([]byte, error) {
			calls.Add(1)
			return fakeUnseal(in)
		}

		plain, err := s.Decrypt(oldSealed)
		if err != nil || plain != "legacy-vpn-password" {
			t.Fatalf("Decrypt legacy key = (%q, %v)", plain, err)
		}
		if calls.Load() != 1 {
			t.Fatalf("expected legacy unseal called once, got %d", calls.Load())
		}

		// 验证磁盘已无感迁移为标准 32 字节跨平台便携格式，全新无 legacy 钩子的 Sealer 即可直接解密。
		migratedRaw, err := os.ReadFile(keyPath)
		if err != nil {
			t.Fatalf("ReadFile migrated secret.key: %v", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(migratedRaw)))
		if err != nil || !bytes.Equal(decoded, realKey) {
			t.Fatalf("migrated key mismatch: len=%d err=%v", len(decoded), err)
		}
		got, err := New(root).Decrypt(oldSealed)
		if err != nil || got != "legacy-vpn-password" {
			t.Fatalf("fresh Sealer Decrypt after migration = (%q, %v)", got, err)
		}
	})

	t.Run("concurrent readers migrate safely", func(t *testing.T) {
		root := t.TempDir()
		if err := storage.WritePrivateFile(storage.SecretKeyPath(root), []byte(legacyEncoded)); err != nil {
			t.Fatalf("write legacy key: %v", err)
		}
		const workers = 6
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for range workers {
			wg.Go(func() {
				s := New(root)
				s.unsealLegacy = fakeUnseal
				got, err := s.Decrypt(oldSealed)
				if err != nil {
					errs <- err
					return
				}
				if got != "legacy-vpn-password" {
					errs <- errors.New("unexpected decrypted password")
				}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent legacy migration failed: %v", err)
		}
		if got, err := New(root).Decrypt(oldSealed); err != nil || got != "legacy-vpn-password" {
			t.Fatalf("post-concurrent Decrypt = (%q, %v)", got, err)
		}
	})

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		t.Run("read-only dir degrades gracefully without breaking decrypt", func(t *testing.T) {
			root := t.TempDir()
			if err := storage.WritePrivateFile(storage.SecretKeyPath(root), []byte(legacyEncoded)); err != nil {
				t.Fatalf("write legacy key: %v", err)
			}
			if err := os.Chmod(root, 0o500); err != nil {
				t.Fatalf("Chmod readonly dir: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

			s := New(root)
			s.unsealLegacy = fakeUnseal
			got, err := s.Decrypt(oldSealed)
			if err != nil || got != "legacy-vpn-password" {
				t.Fatalf("Decrypt in readonly dir = (%q, %v)", got, err)
			}
		})
	}
}

func TestSealerDetectsUnmigratedWindowsDPAPIKey(t *testing.T) {
	root := t.TempDir()
	fakeDPAPI := append(append([]byte(nil), dpapiBlobHeader[:]...), bytes.Repeat([]byte{0xAB}, 64)...)
	if err := storage.WritePrivateFile(storage.SecretKeyPath(root), []byte(base64.StdEncoding.EncodeToString(fakeDPAPI)+"\n")); err != nil {
		t.Fatalf("write synthetic DPAPI key: %v", err)
	}

	s := New(root)
	_, err := s.Decrypt(Sealed{Nonce: "AAAAAAAAAAAAAAAA", Ciphertext: "AAAA"})
	if !errors.Is(err, ErrLegacyKeyMigrationRequired) || !errors.Is(err, ErrMasterKeyInvalid) {
		t.Fatalf("Decrypt synthetic DPAPI key err = %v, want ErrLegacyKeyMigrationRequired & ErrMasterKeyInvalid", err)
	}
	if err := s.EnsurePortable(); !errors.Is(err, ErrLegacyKeyMigrationRequired) {
		t.Fatalf("EnsurePortable synthetic DPAPI key err = %v, want ErrLegacyKeyMigrationRequired", err)
	}
}

func TestSealerEnsurePortableNoOpAndPermissionTightening(t *testing.T) {
	emptyRoot := t.TempDir()
	if err := New(emptyRoot).EnsurePortable(); err != nil {
		t.Fatalf("EnsurePortable on missing key failed: %v", err)
	}
	if _, err := os.Stat(storage.SecretKeyPath(emptyRoot)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("EnsurePortable must not create missing key, stat err=%v", err)
	}

	if runtime.GOOS != "windows" {
		root := t.TempDir()
		s := New(root)
		sealed, err := s.Encrypt("perm-check")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(storage.SecretKeyPath(root), 0o644); err != nil {
			t.Fatal(err)
		}
		if !storage.HasBroadPermissions(storage.SecretKeyPath(root)) {
			t.Fatal("expected 0644 secret.key to report broad permissions")
		}
		if err := New(root).EnsurePortable(); err != nil {
			t.Fatalf("EnsurePortable tighten failed: %v", err)
		}
		if storage.HasBroadPermissions(storage.SecretKeyPath(root)) {
			t.Fatal("expected EnsurePortable to tighten secret.key to 0600")
		}
		if got, err := New(root).Decrypt(sealed); err != nil || got != "perm-check" {
			t.Fatalf("Decrypt after EnsurePortable = (%q, %v)", got, err)
		}
	}
}

func TestSealerConcurrentFirstKeyCreation(t *testing.T) {
	root := t.TempDir()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	sealedList := make([]Sealed, workers)

	for i := range workers {
		wg.Go(func() {
			s := New(root)
			out, err := s.Encrypt("concurrent-secret")
			if err != nil {
				errs <- err
				return
			}
			sealedList[i] = out
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Encrypt failed: %v", err)
	}

	verifier := New(root)
	for i, item := range sealedList {
		got, err := verifier.Decrypt(item)
		if err != nil || got != "concurrent-secret" {
			t.Fatalf("worker %d Decrypt = %q, err = %v", i, got, err)
		}
	}
}

func TestSealerInvalidatesCacheOnKeyRotation(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	first, err := s.Encrypt("before-rotation")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if err := os.Remove(storage.SecretKeyPath(root)); err != nil {
		t.Fatalf("Remove secret.key: %v", err)
	}
	second, err := s.Encrypt("after-rotation")
	if err != nil {
		t.Fatalf("Encrypt after key removal failed: %v", err)
	}
	if _, err := s.Decrypt(first); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("expected ErrDecryptFailed from old rotated key, got %v", err)
	}
	got, err := s.Decrypt(second)
	if err != nil || got != "after-rotation" {
		t.Fatalf("Decrypt after rotation = %q, err = %v", got, err)
	}
}

func TestSealerRejectsSymlinkAndOversizedKeyFile(t *testing.T) {
	t.Run("oversized key file", func(t *testing.T) {
		root := t.TempDir()
		if err := storage.WriteFile(storage.SecretKeyPath(root), []byte(strings.Repeat("A", maxMasterKeyFileBytes+64)), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := New(root).Encrypt("hello"); !errors.Is(err, ErrMasterKeyInvalid) {
			t.Fatalf("expected ErrMasterKeyInvalid for oversized secret.key, got %v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("symlink key file", func(t *testing.T) {
			root := t.TempDir()
			s := New(root)
			if _, err := s.Encrypt("seed"); err != nil {
				t.Fatalf("seed Encrypt: %v", err)
			}
			moved := filepath.Join(root, "real-secret.key")
			if err := os.Rename(storage.SecretKeyPath(root), moved); err != nil {
				t.Fatalf("Rename: %v", err)
			}
			if err := os.Symlink(moved, storage.SecretKeyPath(root)); err != nil {
				t.Fatalf("Symlink: %v", err)
			}
			if _, err := s.Encrypt("after-symlink"); !errors.Is(err, ErrMasterKeyInvalid) {
				t.Fatalf("expected ErrMasterKeyInvalid for symlink secret.key, got %v", err)
			}
		})
	}
}
