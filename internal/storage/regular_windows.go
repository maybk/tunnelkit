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
	f := os.NewFile(uintptr(h), path)
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	return f, nil
}
