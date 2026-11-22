//go:build !darwin && !linux && !windows

package storage

import "os"

func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
