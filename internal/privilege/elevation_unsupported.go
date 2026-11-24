//go:build !darwin && !linux && !windows

package privilege

import "context"

func elevateInstall(context.Context, string) error {
	return unsupported()
}
