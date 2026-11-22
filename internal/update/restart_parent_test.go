package update

import (
	"context"
	"testing"
	"time"
)

func TestWaitForRestartParent(t *testing.T) {
	args := restartArguments()
	if len(args) != 2 || args[0] != restartParentFlag {
		t.Fatalf("unexpected restartArguments(): %v", args)
	}
	if err := WaitForRestartParent(t.Context(), args); err == nil {
		t.Fatal("expected error when parent PID equals current process PID")
	}

	if err := WaitForRestartParent(t.Context(), []string{"--other"}); err != nil {
		t.Fatalf("unexpected error without flag: %v", err)
	}
	if err := WaitForRestartParent(t.Context(), []string{restartParentFlag}); err == nil {
		t.Fatal("expected error for missing parent PID")
	}
	if err := WaitForRestartParent(t.Context(), []string{restartParentFlag, "invalid"}); err == nil {
		t.Fatal("expected error for invalid parent PID")
	}

	calls := 0
	err := waitForParent(t.Context(), 4242, func(pid int) (bool, error) {
		if pid != 4242 {
			t.Fatalf("pid = %d, want 4242", pid)
		}
		calls++
		return calls < 2, nil
	}, 10*time.Millisecond)
	if err != nil || calls != 2 {
		t.Fatalf("waitForParent calls=%d, err=%v", calls, err)
	}

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForParent(canceledCtx, 4242, func(int) (bool, error) { return false, nil }, time.Second); err == nil {
		t.Fatal("expected canceled context during settle delay to return error")
	}
}
