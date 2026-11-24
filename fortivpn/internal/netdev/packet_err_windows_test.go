//go:build windows

package netdev

import "golang.org/x/sys/windows"

func samplePacketRejectedErr() error {
	return windows.WSAEMSGSIZE
}
