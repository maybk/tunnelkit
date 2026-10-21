//go:build !darwin && !windows && !linux

package ipc

import (
	"context"
	"errors"
	"net"
)

var errUnsupportedPlatform = errors.New("ipc: local IPC transport is not supported on this platform")

func CurrentOwnerID() (string, error) {
	return "0", nil
}

func Dial(_ context.Context, _ string) (*Conn, error) {
	return nil, errUnsupportedPlatform
}

func Listen(_ string, _ string) (net.Listener, error) {
	return nil, errUnsupportedPlatform
}
