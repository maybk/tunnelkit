package update

import (
	"context"
	"testing"
)

func TestWaitForRestartParent(t *testing.T) {
	if err := WaitForRestartParent(context.Background(), []string{"--other"}); err != nil {
		t.Fatalf("unexpected error without flag: %v", err)
	}
	if err := WaitForRestartParent(context.Background(), []string{restartParentFlag}); err == nil {
		t.Fatal("expected error for missing parent PID")
	}
	if err := WaitForRestartParent(context.Background(), []string{restartParentFlag, "invalid"}); err == nil {
		t.Fatal("expected error for invalid parent PID")
	}

	calls := 0
	err := waitForParent(context.Background(), 4242, func(pid int) (bool, error) {
		if pid != 4242 {
			t.Fatalf("pid = %d, want 4242", pid)
		}
		calls++
		return calls < 2, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("waitForParent calls=%d, err=%v", calls, err)
	}
}
