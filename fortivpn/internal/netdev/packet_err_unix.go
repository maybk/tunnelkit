//go:build unix

package netdev

import (
	"errors"

	"golang.org/x/sys/unix"
)

// isPacketRejectedErr reports errors that affect only the current packet or transient
// queue pressure rather than permanent TUN device failure.
func isPacketRejectedErr(err error) bool {
	return errors.Is(err, unix.EMSGSIZE) ||
		errors.Is(err, unix.EINVAL) ||
		errors.Is(err, unix.ENOBUFS) ||
		errors.Is(err, unix.EAFNOSUPPORT)
}
