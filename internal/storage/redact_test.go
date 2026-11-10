package storage

import (
	"os"
	"strings"
	"testing"
)

func TestLogTailRedactsLegacyQuotedCredentials(t *testing.T) {
	path := t.TempDir() + "/legacy.log"
	if err := os.WriteFile(path, []byte("INFO password=\"audit old secret\" host=vpn.example.com\nAuthorization: Bearer audit-old-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tail, err := TailFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tail.Content, "audit") {
		t.Error("legacy log exposed synthetic credentials")
	}
	if !strings.Contains(tail.Content, "vpn.example.com") {
		t.Error("non-sensitive diagnostic was removed")
	}
}

func TestLogTailOmitsFragmentWithoutCredentialKey(t *testing.T) {
	path := t.TempDir() + "/fragment.log"
	if err := os.WriteFile(path, []byte(`password="audit-secret-suffix"`), 0600); err != nil {
		t.Fatal(err)
	}
	tail, err := TailFile(path, 16)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tail.Content, "suffix") {
		t.Error("truncated credential fragment was exposed")
	}
}
