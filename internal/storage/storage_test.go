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

	// Verify inline structured log redaction and multi-byte UTF-8 boundary truncation.
	structured := strings.Repeat("前缀中文日志行-测试截断\n", 10) +
		"2026-10-22 10:00:00 INFO  Auth (user=alice token=sec999 password=pwd888 host=vpn.example.com)\n"
	if err := os.WriteFile(logPath, []byte(structured), 0600); err != nil {
		t.Fatalf("WriteFile structured: %v", err)
	}
	tail2, err := TailFile(logPath, 160)
	if err != nil {
		t.Fatalf("TailFile truncated: %v", err)
	}
	if strings.Contains(tail2.Content, "sec999") || strings.Contains(tail2.Content, "pwd888") {
		t.Fatalf("TailFile did not redact inline structured secrets: %s", tail2.Content)
	}
	if !strings.Contains(tail2.Content, "user=alice") || !strings.Contains(tail2.Content, "host=vpn.example.com") {
		t.Fatalf("TailFile wiped non-sensitive fields: %s", tail2.Content)
	}
}
