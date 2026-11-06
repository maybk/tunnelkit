//go:build !darwin && !linux && !windows

package privilege

import "os"

func lockFile(*os.File) error {
	return unsupported()
}
