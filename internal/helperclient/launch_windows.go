//go:build windows

package helperclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func defaultEndpoint() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return `\\.\pipe\TunnelKit-` + hex.EncodeToString(b[:]), nil
}

func launchElevated(
	_ context.Context,
	helperPath string,
	endpoint string,
	ownerID string,
	tokenSHA256 string,
	logPath string,
	journalPath string,
) error {
	args := []string{
		"serve",
		"--endpoint", winQuoteArg(endpoint),
		"--sid", winQuoteArg(ownerID),
		"--token-sha256", winQuoteArg(tokenSHA256),
	}
	if logPath != "" {
		args = append(args, "--log", winQuoteArg(logPath))
	}
	if journalPath != "" {
		args = append(args, "--journal", winQuoteArg(journalPath))
	}
	paramStr := strings.Join(args, " ")

	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := windows.UTF16PtrFromString(helperPath)
	if err != nil {
		return err
	}
	paramPtr, err := windows.UTF16PtrFromString(paramStr)
	if err != nil {
		return err
	}
	dirPtr, err := windows.UTF16PtrFromString(filepath.Dir(helperPath))
	if err != nil {
		return err
	}

	err = windows.ShellExecute(0, verbPtr, filePtr, paramPtr, dirPtr, windows.SW_HIDE)
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("Windows UAC elevation failed: %w", err)
	}
	return nil
}

func winQuoteArg(s string) string {
	return windows.EscapeArg(s)
}
