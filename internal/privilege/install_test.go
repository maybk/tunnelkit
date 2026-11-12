package privilege

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWithStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("skips elevation when already current", func(t *testing.T) {
		t.Parallel()
		elevated := 0
		err := ensureWith(
			context.Background(),
			"helper",
			func(string) (string, error) { return "", nil },
			func(context.Context, string) error {
				elevated++
				return nil
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elevated != 0 {
			t.Fatalf("expected 0 elevations, got %d", elevated)
		}
	})

	t.Run("repairs recoverable drift via single elevation", func(t *testing.T) {
		t.Parallel()
		checks := 0
		elevated := 0
		err := ensureWith(
			context.Background(),
			"helper",
			func(string) (string, error) {
				checks++
				if checks == 1 {
					return "系统服务未注册", nil
				}
				return "", nil
			},
			func(context.Context, string) error {
				elevated++
				return nil
			},
		)
		if err != nil {
			t.Fatalf("expected self-heal to succeed, got %v", err)
		}
		if elevated != 1 || checks != 2 {
			t.Fatalf("unexpected counts: elevated=%d checks=%d", elevated, checks)
		}
	})

	t.Run("aborts before elevation on hard security error", func(t *testing.T) {
		t.Parallel()
		hardErr := errors.New("helper paths must not be links")
		elevated := 0
		err := ensureWith(
			context.Background(),
			"helper",
			func(string) (string, error) { return "", hardErr },
			func(context.Context, string) error {
				elevated++
				return nil
			},
		)
		if !errors.Is(err, hardErr) {
			t.Fatalf("expected hard error %v, got %v", hardErr, err)
		}
		if elevated != 0 {
			t.Fatalf("must not elevate on hard security violation, elevated=%d", elevated)
		}
	})

	t.Run("preserves ErrCancelled from elevation", func(t *testing.T) {
		t.Parallel()
		err := ensureWith(
			context.Background(),
			"helper",
			func(string) (string, error) { return "系统辅助程序未安装", nil },
			func(context.Context, string) error { return ErrCancelled },
		)
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("expected ErrCancelled, got %v", err)
		}
	})

	t.Run("surfaces diagnostic reason if post-install check still mismatches", func(t *testing.T) {
		t.Parallel()
		err := ensureWith(
			context.Background(),
			"helper",
			func(string) (string, error) { return "服务安全访问控制列表未就绪", nil },
			func(context.Context, string) error { return nil },
		)
		if err == nil || !strings.Contains(err.Error(), "服务安全访问控制列表未就绪") {
			t.Fatalf("expected diagnostic mismatch reason in error, got %v", err)
		}
	})
}

func TestDigestRejectsNonRegularFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := digest(dir); err == nil {
		t.Fatal("expected digest to reject directory")
	}

	fileA := filepath.Join(dir, "a.bin")
	fileB := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(fileA, []byte("tunnelkit-helper-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("tunnelkit-helper-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	sumA, err := digest(fileA)
	if err != nil {
		t.Fatalf("digest A: %v", err)
	}
	sumB, err := digest(fileB)
	if err != nil {
		t.Fatalf("digest B: %v", err)
	}
	if sumA != sumB {
		t.Fatalf("expected identical SHA-256 digests, got %x != %x", sumA, sumB)
	}
}
