//go:build !darwin && !windows && !linux

package netcfg

import (
	"context"

	"tunnelkit/fortivpn"
)

// Apply returns CodePlatformUnsupported on unsupported platforms.
func Apply(_ context.Context, _ Desired, _ string) (Applied, error) {
	return nil, fortivpn.NewError(fortivpn.CodeUnsupportedPlatform, "netcfg: host network configuration is not supported on this platform")
}

// Recover is a no-op on unsupported platforms.
func Recover(_ context.Context, journalPath string) error {
	j, err := LoadJournal(journalPath)
	if err != nil {
		return err
	}
	return j.Clear()
}
