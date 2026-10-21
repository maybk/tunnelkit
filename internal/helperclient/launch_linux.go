//go:build linux

package helperclient

import (
	"context"
	"errors"
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

	helperArgs := []string{
		"serve",
		"--endpoint", endpoint,
		"--uid", ownerID,
		"--token-sha256", tokenSHA256,
	}
	if logPath != "" {
		helperArgs = append(helperArgs, "--log", logPath)
	}
	if journalPath != "" {
		helperArgs = append(helperArgs, "--journal", journalPath)
	}

	// If already running as root, spawn the helper directly in the background.
	if os.Geteuid() == 0 {
		cmd := exec.Command(helperPath, helperArgs...)
		return cmd.Start()
	}

	// Use PolicyKit pkexec (standard on Linux desktops) to launch the helper in the background.
	if pkexecPath, err := exec.LookPath("pkexec"); err == nil {
		shArgs := append([]string{"/bin/sh", "-c", `"$@" >/dev/null 2>&1 &`, "sh", helperPath}, helperArgs...)
		cmd := exec.CommandContext(ctx, pkexecPath, shArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			text := strings.TrimSpace(string(out))
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && (exitErr.ExitCode() == 126 || exitErr.ExitCode() == 127) {
				return ErrElevationCancelled
			}
			lower := strings.ToLower(text)
			if strings.Contains(lower, "dismissed") || strings.Contains(lower, "not authorized") || strings.Contains(lower, "canceled") || strings.Contains(lower, "cancelled") {
				return ErrElevationCancelled
			}
			return fmt.Errorf("linux pkexec elevation failed: %s (%w)", text, err)
		}
		return nil
	}

	// Fallback to passwordless sudo -n when pkexec is unavailable.
	if sudoPath, err := exec.LookPath("sudo"); err == nil {
		sudoArgs := append([]string{"-n", "-b", helperPath}, helperArgs...)
		out, err := exec.CommandContext(ctx, sudoPath, sudoArgs...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("linux sudo -n elevation failed: %s (%w)", strings.TrimSpace(string(out)), err)
		}
		return nil
	}

	return errors.New("linux elevation requires pkexec (PolicyKit) or passwordless sudo")
}
