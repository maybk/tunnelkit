//go:build darwin

package helperclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func defaultEndpoint() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tunnelkit", "run", "helper.sock"), nil
}

func launchElevated(
	ctx context.Context,
	helperPath string,
	endpoint string,
	ownerID string,
	tokenSHA256 string,
	logPath string,
	journalPath string,
) error {
	if err := os.MkdirAll(filepath.Dir(endpoint), 0o700); err != nil {
		return err
	}

	args := []string{
		shellQuote(helperPath),
		"'serve'",
		"'--endpoint'", shellQuote(endpoint),
		"'--uid'", shellQuote(ownerID),
		"'--token-sha256'", shellQuote(tokenSHA256),
	}
	if logPath != "" {
		args = append(args, "'--log'", shellQuote(logPath))
	}
	if journalPath != "" {
		args = append(args, "'--journal'", shellQuote(journalPath))
	}
	args = append(args, ">/dev/null 2>&1 &")

	shellCommand := strings.Join(args, " ")
	script := "do shell script " + appleScriptString(shellCommand) + " with administrator privileges"
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if strings.Contains(text, "(-128)") || strings.Contains(text, "User canceled") || strings.Contains(text, "用户已取消") {
			return ErrElevationCancelled
		}
		return fmt.Errorf("macOS elevation failed: %s", text)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func appleScriptString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}
