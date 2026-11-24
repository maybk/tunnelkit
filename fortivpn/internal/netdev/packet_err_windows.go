//go:build windows

package netdev

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isPacketRejectedErr reports errors that affect only the current packet or transient
// Wintun ring pressure rather than permanent adapter failure.
func isPacketRejectedErr(err error) bool {
	return errors.Is(err, windows.WSAEMSGSIZE) ||
		errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) ||
		errors.Is(err, windows.ERROR_INVALID_PARAMETER)
}
