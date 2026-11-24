//go:build !unix && !windows

package storage

import "os"

func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
