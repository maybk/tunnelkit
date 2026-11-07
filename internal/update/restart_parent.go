package update

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

const restartParentFlag = "--wait-for-parent"

func restartArguments() []string { return []string{restartParentFlag, strconv.Itoa(os.Getpid())} }

// WaitForRestartParent 必须在创建 GUI、取得单实例锁或清理备份之前调用。
func WaitForRestartParent(ctx context.Context, args []string) error {
	for i, arg := range args {
		if arg != restartParentFlag {
			continue
		}
		if i+1 >= len(args) {
			return fmt.Errorf("missing parent PID argument for %s", restartParentFlag)
		}
		pid, err := strconv.Atoi(args[i+1])
		if err != nil || pid <= 1 || pid == os.Getpid() {
			return fmt.Errorf("invalid parent PID argument for %s", restartParentFlag)
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		return waitForParent(ctx, pid, processAlive)
	}
	return nil
}

func waitForParent(ctx context.Context, pid int, alive func(int) (bool, error)) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		running, err := alive(pid)
		if err != nil {
			return fmt.Errorf("wait for previous process exit: %w", err)
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for previous process exit: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
