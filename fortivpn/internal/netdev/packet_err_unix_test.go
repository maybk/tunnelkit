//go:build unix

package netdev

import "golang.org/x/sys/unix"

func samplePacketRejectedErr() error {
	return unix.EMSGSIZE
}
