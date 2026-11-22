package storage

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadRegularFileBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	want := []byte("hello-regular-file")
	if err := WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, info, err := ReadRegularFileBounded(path, int64(len(want)))
	if err != nil {
		t.Fatalf("ReadRegularFileBounded: %v", err)
	}
	if !bytes.Equal(got, want) || info == nil || !info.Mode().IsRegular() {
		t.Fatalf("ReadRegularFileBounded = (%q, %v), want %q", got, info, want)
	}

	if _, _, err := ReadRegularFileBounded(filepath.Join(dir, "missing.txt"), 64); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file err = %v, want fs.ErrNotExist", err)
	}

	if _, _, err := ReadRegularFileBounded(path, int64(len(want)-1)); !errors.Is(err, ErrFileTooLarge) || !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("oversized file err = %v, want ErrFileTooLarge + os.ErrInvalid", err)
	}

	if _, _, err := ReadRegularFileBounded(dir, 64); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("directory err = %v, want os.ErrInvalid", err)
	}
	if f, err := OpenRegularNoFollow(dir); !errors.Is(err, os.ErrInvalid) {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("OpenRegularNoFollow(dir) err = %v, want os.ErrInvalid", err)
	}

	if runtime.GOOS != "windows" {
		linkPath := filepath.Join(dir, "link.txt")
		if err := os.Symlink(path, linkPath); err != nil {
			t.Fatalf("Symlink: %v", err)
		}
		if _, _, err := ReadRegularFileBounded(linkPath, 64); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("symlink err = %v, want os.ErrInvalid", err)
		}
		if f, err := OpenRegularNoFollow(linkPath); err == nil {
			_ = f.Close()
			t.Fatal("OpenRegularNoFollow must reject symlink")
		}
		if _, err := TailFile(linkPath, 64); err == nil {
			t.Fatal("TailFile must reject symlink")
		}
	}
}
