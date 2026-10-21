//go:build !darwin && !windows && !linux

package helperclient

import (
	"context"
	"errors"
)

func defaultEndpoint() (string, error) {
	return "", errors.New("helper launch is not supported on this platform")
}

func launchElevated(_ context.Context, _, _, _, _, _, _ string) error {
	return errors.New("helper launch is not supported on this platform")
}
