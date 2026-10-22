//go:build linux

package helperclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}

	// Use PolicyKit pkexec (standard on Linux desktops) to launch the helper directly.
	if pkexecPath, err := exec.LookPath("pkexec"); err == nil {
		pkArgs := append([]string{helperPath}, helperArgs...)
		cmd := exec.Command(pkexecPath, pkArgs...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err == nil {
			waitCh := make(chan error, 1)
			go func() {
				waitCh <- cmd.Wait()
			}()

			ticker := time.NewTicker(40 * time.Millisecond)
			defer ticker.Stop()
			timeout := time.NewTimer(2 * time.Minute)
			defer timeout.Stop()

			for {
				if _, statErr := os.Lstat(endpoint); statErr == nil {
					return nil
				}
				select {
				case wErr := <-waitCh:
					if _, statErr := os.Lstat(endpoint); statErr == nil {
						return nil
					}
					text := strings.TrimSpace(stderr.String())
					lower := strings.ToLower(text)
					var exitErr *exec.ExitError
					if errors.As(wErr, &exitErr) && exitErr.ExitCode() == 126 {
						return ErrElevationCancelled
					}
					if strings.Contains(lower, "dismissed") || strings.Contains(lower, "canceled") || strings.Contains(lower, "cancelled") {
						return ErrElevationCancelled
					}
					// If pkexec exited with 127 or no Polkit agent is available, fall back to sudo -n.
					if (errors.As(wErr, &exitErr) && exitErr.ExitCode() == 127) ||
						strings.Contains(lower, "agent") ||
						strings.Contains(lower, "authority") {
						goto trySudo
					}
					if strings.Contains(lower, "not authorized") {
						return ErrElevationCancelled
					}
					return fmt.Errorf("linux pkexec elevation failed: %s (%w)", text, wErr)
				case <-timeout.C:
					_ = cmd.Process.Kill()
					return errors.New("linux pkexec elevation timed out")
				case <-ctx.Done():
					_ = cmd.Process.Kill()
					return ctx.Err()
				case <-ticker.C:
				}
			}
		}
	}

trySudo:
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
