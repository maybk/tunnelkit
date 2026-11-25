//go:build !windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixRestrictPrivateFileAndBroadPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.key")
	if err := os.WriteFile(path, []byte("synthetic\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !HasBroadPermissions(path) {
		t.Fatal("expected 0644 file to report broad permissions")
	}
	if err := RestrictPrivateFile(path); err != nil {
		t.Fatalf("RestrictPrivateFile: %v", err)
	}
	if HasBroadPermissions(path) {
		t.Fatal("expected tightened 0600 file not to report broad permissions")
	}
	if err := RestrictPrivateFile(dir); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("RestrictPrivateFile(dir) err = %v, want os.ErrInvalid", err)
	}

	// 验证对符号链接拒绝收紧且绝不篡改目标文件权限。
	target := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	link := filepath.Join(dir, "link.key")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if HasBroadPermissions(link) {
		t.Fatal("expected symlink not to report broad permissions")
	}
	if err := RestrictPrivateFile(link); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("RestrictPrivateFile(symlink) err = %v, want os.ErrInvalid", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat target: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target permission mutated to %04o, want 0644", info.Mode().Perm())
	}
}
