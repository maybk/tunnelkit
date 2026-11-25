//go:build !unix && !windows

package storage

import "os"

func openNoFollow(path string) (*os.File, error) {
	linfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if linfo.Mode()&os.ModeSymlink != 0 || !linfo.Mode().IsRegular() {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(linfo, info) {
		_ = f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	return f, nil
}
