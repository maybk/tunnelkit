//go:build !unix && !windows

package privilege

import "os"

func lockFile(*os.File) error {
	return unsupported()
}
