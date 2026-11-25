package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultRootUsesHomeDotTunnelkit(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	root, err := DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	want := filepath.Join(home, ".tunnelkit")
	if root != want {
		t.Fatalf("DefaultRoot() = %q, want %q", root, want)
	}
}

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
	wantPerm := os.FileMode(0600)
	if runtime.GOOS == "windows" {
		wantPerm = 0666
	}
	if info.Mode().Perm() != wantPerm {
		t.Fatalf("expected perm %04o, got %04o", wantPerm, info.Mode().Perm())
	}
}

func TestWriteFileWithHookAndPrivateFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "private.json")

	if err := WritePrivateFile(target, []byte("initial\n")); err != nil {
		t.Fatalf("WritePrivateFile initial failed: %v", err)
	}
	if HasBroadPermissions(target) {
		t.Fatal("expected WritePrivateFile target not to have broad permissions")
	}

	var seenTmp string
	hookErr := os.ErrPermission
	if err := WriteFileWithHook(target, []byte("should-not-commit\n"), 0o600, func(tmpPath string) error {
		seenTmp = tmpPath
		if tmpPath == target {
			t.Fatal("beforeRename hook must run on temp path before rename")
		}
		return hookErr
	}); !errors.Is(err, hookErr) {
		t.Fatalf("expected hook error %v, got %v", hookErr, err)
	}
	if _, statErr := os.Stat(seenTmp); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp file %q to be cleaned up on hook error, stat err=%v", seenTmp, statErr)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "initial\n" {
		t.Fatalf("original file changed on hook failure: (%q, %v)", string(got), err)
	}

	// 验证目标文件被设为只读后，WritePrivateFile 仍可原子覆盖。
	if err := os.Chmod(target, 0o400); err != nil {
		t.Fatalf("Chmod readonly failed: %v", err)
	}
	if err := WritePrivateFile(target, []byte("replaced\n")); err != nil {
		t.Fatalf("WritePrivateFile over readonly target failed: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "replaced\n" {
		t.Fatalf("ReadFile after readonly replace = (%q, %v)", string(got), err)
	}
	if HasBroadPermissions(target) {
		t.Fatal("expected replaced private file not to have broad permissions")
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

	// Verify truncated single huge line drops unkeyed secret tail without trailing newline.
	hugePath := filepath.Join(dir, "huge.log")
	if err := os.WriteFile(hugePath, []byte("password="+strings.Repeat("S", 512)), 0600); err != nil {
		t.Fatalf("WriteFile huge: %v", err)
	}
	tailHuge, err := TailFile(hugePath, 128)
	if err != nil {
		t.Fatalf("TailFile huge: %v", err)
	}
	if strings.Contains(tailHuge.Content, "SSSS") {
		t.Fatalf("TailFile leaked secret tail from truncated line: %q", tailHuge.Content)
	}
	if tailHuge.Content != "... 已省略更早的日志" {
		t.Fatalf("TailFile huge content = %q, want clean omission notice without trailing newline", tailHuge.Content)
	}
}
