//go:build linux

package ipc

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func verifyPeerUID(conn *net.UnixConn, expectedUID uint32) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var (
		cred *unix.Ucred
		sErr error
	)
	if err := rawConn.Control(func(fd uintptr) {
		cred, sErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if sErr != nil {
		return sErr
	}
	if cred.Uid != expectedUID && cred.Uid != 0 {
		return fmt.Errorf("ipc: peer UID %d does not match expected UID %d", cred.Uid, expectedUID)
	}
	return nil
}
