//go:build !darwin && !windows && !linux

package update

import "context"

func applyPlatformUpdate(_ context.Context, _ string, _ ProgressFunc) (func() error, error) {
	return nil, ErrSelfUpdateNotReady
}
