//go:build windows

package storage

import (
	"os"

	"golang.org/x/sys/windows"
)

func openNoFollow(path string) (*os.File, error) {
	h, err := openRegularHandleNoFollow(path, windows.GENERIC_READ)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
