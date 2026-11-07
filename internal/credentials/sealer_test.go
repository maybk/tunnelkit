package credentials

import (
	"os"
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
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s := New(root)
			out, err := s.Encrypt("concurrent-secret")
			if err != nil {
				errs <- err
				return
			}
			sealedList[idx] = out
		}(i)
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
