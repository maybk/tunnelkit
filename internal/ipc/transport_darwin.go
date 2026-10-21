//go:build darwin

package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// CurrentOwnerID returns the current process's real numeric UID on macOS.
func CurrentOwnerID() (string, error) {
	return strconv.Itoa(os.Getuid()), nil
}

// Dial connects to the helper's Unix domain socket.
func Dial(ctx context.Context, endpoint string) (*Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	raw, err := d.DialContext(ctx, "unix", endpoint)
	if err != nil {
		return nil, err
	}
	return NewConn(raw), nil
}

// Listen creates a Unix domain socket at endpoint owned by ownerUID (mode 0600)
// and verifies connecting peers via LOCAL_PEERCRED.
func Listen(endpoint string, ownerUID string) (net.Listener, error) {
	uid, err := strconv.Atoi(strings.TrimSpace(ownerUID))
	if err != nil || uid < 0 {
		return nil, fmt.Errorf("ipc: invalid owner UID %q", ownerUID)
	}

	if err := os.MkdirAll(filepath.Dir(endpoint), 0o700); err != nil {
		return nil, fmt.Errorf("ipc: create socket directory: %w", err)
	}

	if _, statErr := os.Lstat(endpoint); statErr == nil {
		// Check whether an existing helper is actively listening on this socket.
		testConn, dialErr := net.DialTimeout("unix", endpoint, 200*time.Millisecond)
		if dialErr == nil {
			_ = testConn.Close()
			return nil, fmt.Errorf("ipc: helper socket %s is already in use", endpoint)
		}
		if err := os.Remove(endpoint); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("ipc: remove stale socket: %w", err)
		}
	}

	ln, err := net.Listen("unix", endpoint)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen unix %s: %w", endpoint, err)
	}

	if err := os.Chmod(endpoint, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("ipc: chmod socket: %w", err)
	}
	// If running as root, chown the socket to the unprivileged GUI user UID.
	if os.Geteuid() == 0 && uid != 0 {
		if err := os.Chown(endpoint, uid, -1); err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("ipc: chown socket to uid %d: %w", uid, err)
		}
	}

	return &darwinListener{
		Listener: ln,
		endpoint: endpoint,
		ownerUID: uint32(uid),
	}, nil
}

type darwinListener struct {
	net.Listener
	endpoint string
	ownerUID uint32
}

func (l *darwinListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uconn, ok := conn.(*net.UnixConn)
		if !ok {
			_ = conn.Close()
			continue
		}
		if err := verifyPeerUID(uconn, l.ownerUID); err != nil {
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}

func (l *darwinListener) Close() error {
	err := l.Listener.Close()
	_ = os.Remove(l.endpoint)
	return err
}

func verifyPeerUID(conn *net.UnixConn, expectedUID uint32) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var (
		cred *unix.Xucred
		sErr error
	)
	if err := rawConn.Control(func(fd uintptr) {
		cred, sErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
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
