package storage

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
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

	missing := filepath.Join(dir, "missing.txt")
	if _, _, err := ReadRegularFileBounded(missing, 64); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file err = %v, want fs.ErrNotExist", err)
	}
	if f, err := OpenRegularNoFollow(missing); !errors.Is(err, fs.ErrNotExist) {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("OpenRegularNoFollow(missing) err = %v, want fs.ErrNotExist", err)
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

	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(path, linkPath); err == nil {
		if _, _, err := ReadRegularFileBounded(linkPath, 64); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("symlink err = %v, want os.ErrInvalid", err)
		}
		if f, err := OpenRegularNoFollow(linkPath); !errors.Is(err, os.ErrInvalid) {
			if f != nil {
				_ = f.Close()
			}
			t.Fatalf("OpenRegularNoFollow(symlink) err = %v, want os.ErrInvalid", err)
		}
		if _, err := TailFile(linkPath, 64); err == nil {
			t.Fatal("TailFile must reject symlink")
		}
		danglingLink := filepath.Join(dir, "dangling.txt")
		if err := os.Symlink(filepath.Join(dir, "absent.txt"), danglingLink); err != nil {
			t.Fatalf("dangling Symlink: %v", err)
		}
		if f, err := OpenRegularNoFollow(danglingLink); !errors.Is(err, os.ErrInvalid) {
			if f != nil {
				_ = f.Close()
			}
			t.Fatalf("OpenRegularNoFollow(dangling symlink) err = %v, want os.ErrInvalid", err)
		}
	} else if runtime.GOOS != "windows" {
		t.Fatalf("Symlink: %v", err)
	}
}

func TestReadRegularFileBoundedConcurrentAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "profiles.json")
	payloadA := `{"schemaVersion":1,"data":{"state":"alpha"}}` + "\n"
	payloadB := `{"schemaVersion":1,"data":{"state":"bravo-longer"}}` + "\n"

	if err := WritePrivateFile(target, []byte(payloadA)); err != nil {
		t.Fatalf("initial WritePrivateFile: %v", err)
	}

	// 验证高频原子替换与并发读取之间不存在 Lstat/open TOCTOU 导致的 os.ErrInvalid 误报。
	const iterations = 60
	var done atomic.Bool
	var reads atomic.Int64
	var writes atomic.Int64
	var wg sync.WaitGroup
	errCh := make(chan error, 8)

	for range 4 {
		wg.Go(func() {
			for !done.Load() {
				raw, info, err := ReadRegularFileBounded(target, 4096)
				if err != nil {
					if errors.Is(err, os.ErrInvalid) || runtime.GOOS != "windows" {
						select {
						case errCh <- err:
						default:
						}
						return
					}
					continue
				}
				if info == nil || (string(raw) != payloadA && string(raw) != payloadB) {
					select {
					case errCh <- errors.New("unexpected partial or mismatched payload during concurrent replace"):
					default:
					}
					return
				}
				reads.Add(1)
			}
		})
	}

	for i := range iterations {
		body := payloadA
		if i%2 == 1 {
			body = payloadB
		}
		if err := WritePrivateFile(target, []byte(body)); err != nil {
			if runtime.GOOS != "windows" {
				done.Store(true)
				wg.Wait()
				t.Fatalf("WritePrivateFile iteration %d: %v", i, err)
			}
			continue
		}
		writes.Add(1)
	}
	done.Store(true)
	wg.Wait()

	select {
	case err := <-errCh:
		t.Fatalf("concurrent ReadRegularFileBounded failed: %v", err)
	default:
	}
	if reads.Load() == 0 || writes.Load() == 0 {
		t.Fatalf("expected concurrent reads and writes to execute, got reads=%d writes=%d", reads.Load(), writes.Load())
	}
}
