package credentials

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"tunnelkit/internal/storage"
)

func TestSealerRoundTripAndKeyPersistence(t *testing.T) {
	root := t.TempDir()
	s1 := New(root)
	sealed, err := s1.Encrypt("top-secret-value")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if sealed.Nonce == "" || sealed.Ciphertext == "" {
		t.Fatalf("expected non-empty sealed payload: %+v", sealed)
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

	if _, err := os.Stat(storage.SecretKeyPath(root)); err != nil {
		t.Fatalf("secret.key should exist: %v", err)
	}
}

func TestSealerRejectsInvalidNonceAndCiphertext(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Decrypt(Sealed{Nonce: "bad-base64!", Ciphertext: "AA=="}); err == nil {
		t.Fatal("expected invalid base64 nonce error")
	}
	if _, err := s.Decrypt(Sealed{Nonce: "AA==", Ciphertext: "AA=="}); err == nil {
		t.Fatal("expected invalid nonce length error")
	}
	sealed, err := s.Encrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	sealed.Ciphertext = "AAAA"
	if _, err := s.Decrypt(sealed); err == nil {
		t.Fatal("expected tampered ciphertext error")
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
	if _, err := s.Decrypt(first); err == nil {
		t.Fatal("expected ciphertext from old rotated key to fail decryption")
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
		if _, err := New(root).Encrypt("hello"); err == nil {
			t.Fatal("expected oversized secret.key to be rejected")
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
			if _, err := s.Encrypt("after-symlink"); err == nil {
				t.Fatal("expected symlink secret.key to be rejected")
			}
		})
	}
}
