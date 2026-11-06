package ipc

import (
	"errors"
	"io/fs"
	"syscall"
)

// EndpointUnavailable identifies a missing listener, not a timeout, failed
// authentication, or uncertain transport failure. Only the former is stale.
func EndpointUnavailable(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED)
}
