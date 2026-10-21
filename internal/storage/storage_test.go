package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicAndPerm(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "data.json")

	if err := WriteFile(target, []byte("first"), 0600); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := WriteFile(target, []byte("second"), 0600); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("unexpected content: %q", string(got))
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected perm 0600, got %o", info.Mode().Perm())
	}
}

func TestTailAndResetFile(t *testing.T) {
	dir := t.TempDir()
	logPath := LogPath(dir, "abc")

	if err := ResetFile(logPath); err != nil {
		t.Fatalf("ResetFile: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("password = mysecret\nCookie: SVPNCOOKIE=tok123\nINFO: Tunnel is up and running.\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tail, err := TailFile(logPath, 1024)
	if err != nil {
		t.Fatalf("TailFile: %v", err)
	}
	if !tail.Exists {
		t.Fatalf("expected tail.Exists=true")
	}
	if strings.Contains(tail.Content, "mysecret") || strings.Contains(tail.Content, "tok123") {
		t.Fatalf("TailFile did not redact secrets: %s", tail.Content)
	}
}
