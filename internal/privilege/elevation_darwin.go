//go:build darwin

package privilege

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func elevateInstall(ctx context.Context, source string) error {
	if os.Geteuid() == 0 {
		return Install(source)
	}
	command := shellQuoteCommand(source, []string{"install"})
	script := "do shell script " + appleScriptString(command) + " with administrator privileges"
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := strings.TrimSpace(string(out))
		if strings.Contains(text, "(-128)") {
			return ErrCancelled
		}
		return fmt.Errorf("辅助组件安装失败: %s", text)
	}
	return nil
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}
