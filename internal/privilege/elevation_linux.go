//go:build linux

package privilege

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunElevatedLinux executes bin with args as root via PolicyKit pkexec, falling
// back to non-interactive sudo -n only when no PolicyKit authentication agent is
// available. Returns ErrCancelled if the user dismisses the authorization dialog.
func RunElevatedLinux(ctx context.Context, bin string, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pkexec, err := exec.LookPath("pkexec"); err == nil {
		pkArgs := append([]string{bin}, args...)
		out, runErr := exec.CommandContext(ctx, pkexec, pkArgs...).CombinedOutput()
		if runErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := strings.TrimSpace(string(out))
		lower := strings.ToLower(text)
		var exit *exec.ExitError
		if (errors.As(runErr, &exit) && exit.ExitCode() == 126) ||
			strings.Contains(lower, "dismissed") ||
			strings.Contains(lower, "cancel") {
			return ErrCancelled
		}
		// 若桌面环境未运行 PolicyKit 代理（退出码 127 或提示无 agent），可回退至免密 sudo；
		// 真实执行失败直接返回而不触发二次弹窗。
		noAgent := (errors.As(runErr, &exit) && exit.ExitCode() == 127) || strings.Contains(lower, "agent")
		if !noAgent {
			return fmt.Errorf("%s (%w)", text, runErr)
		}
	}
	if sudo, err := exec.LookPath("sudo"); err == nil {
		sudoArgs := append([]string{"-n", bin}, args...)
		out, runErr := exec.CommandContext(ctx, sudo, sudoArgs...).CombinedOutput()
		if runErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s (%w)", strings.TrimSpace(string(out)), runErr)
	}
	return errors.New("需要 PolicyKit 授权代理或已授权的 sudo")
}

func elevateInstall(ctx context.Context, source string) error {
	if os.Geteuid() == 0 {
		return Install(source)
	}
	if err := RunElevatedLinux(ctx, source, "install"); err != nil {
		if errors.Is(err, ErrCancelled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("辅助组件安装失败: %w", err)
	}
	return nil
}
