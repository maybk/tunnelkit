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

func elevateInstall(ctx context.Context, source string) error {
	if os.Geteuid() == 0 {
		return Install(source)
	}
	if pkexec, err := exec.LookPath("pkexec"); err == nil {
		out, err := exec.CommandContext(ctx, pkexec, source, "install").CombinedOutput()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 126 {
			return ErrCancelled
		}
		text := strings.TrimSpace(string(out))
		// Missing agents may use a pre-authorized sudo rule. Real installation
		// failures are returned, rather than triggering a second authorization.
		if !strings.Contains(strings.ToLower(text), "agent") {
			return fmt.Errorf("辅助组件安装失败: %s (%w)", text, err)
		}
	}
	if sudo, err := exec.LookPath("sudo"); err == nil {
		out, err := exec.CommandContext(ctx, sudo, "-n", source, "install").CombinedOutput()
		if err != nil {
			return fmt.Errorf("辅助组件安装需要管理员授权: %s (%w)", strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	return errors.New("安装辅助组件需要 PolicyKit 授权代理或已授权的 sudo")
}
