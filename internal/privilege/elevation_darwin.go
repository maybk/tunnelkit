//go:build darwin

package privilege

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunElevatedDarwinShell runs shellCmd via /usr/bin/osascript "do shell script ... with administrator privileges".
// Returns ErrCancelled if the user cancels the authorization dialog.
func RunElevatedDarwinShell(ctx context.Context, shellCmd string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	script := "do shell script " + appleScriptString(shellCmd) + " with administrator privileges"
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := strings.TrimSpace(string(out))
		lower := strings.ToLower(text)
		if strings.Contains(lower, "(-128)") ||
			strings.Contains(lower, "canceled") ||
			strings.Contains(lower, "cancelled") ||
			strings.Contains(text, "用户已取消") {
			return ErrCancelled
		}
		return fmt.Errorf("%s (%w)", text, err)
	}
	return nil
}

func elevateInstall(ctx context.Context, source string) error {
	if os.Geteuid() == 0 {
		return Install(source)
	}
	command := ShellQuoteCommand(source, []string{"install"})
	if err := RunElevatedDarwinShell(ctx, command); err != nil {
		if errors.Is(err, ErrCancelled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("辅助组件安装失败: %w", err)
	}
	return nil
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}
