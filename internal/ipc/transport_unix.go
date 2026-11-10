//go:build darwin || linux

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
	"sync"
	"time"
)

// CurrentOwnerID returns the current process's real numeric UID on Unix platforms.
func CurrentOwnerID() (string, error) {
	return strconv.Itoa(os.Getuid()), nil
}

// Dial connects to the helper's Unix domain socket after verifying it is a socket file.
func Dial(ctx context.Context, endpoint string) (*Conn, error) {
	if err := verifySocketFile(endpoint); err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	raw, err := d.DialContext(ctx, "unix", endpoint)
	if err != nil {
		return nil, err
	}
	return NewConn(raw), nil
}

// Listen creates a Unix domain socket at endpoint owned by ownerUID (mode 0600)
// and verifies connecting peers via platform-specific socket peer credentials.
func Listen(endpoint string, ownerUID string) (net.Listener, error) {
	uid, err := strconv.Atoi(strings.TrimSpace(ownerUID))
	if err != nil || uid < 0 {
		return nil, fmt.Errorf("ipc: invalid owner UID %q", ownerUID)
	}

	if err := validateSocketDir(filepath.Dir(endpoint)); err != nil {
		return nil, err
	}
	if err := cleanupStaleSocket(endpoint); err != nil {
		return nil, err
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

	return &unixListener{
		Listener: ln,
		endpoint: endpoint,
		ownerUID: uint32(uid),
	}, nil
}

func validateSocketDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("ipc: create socket directory: %w", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("ipc: stat socket directory: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("ipc: socket directory %s is not a directory", dir)
	}
	return nil
}

func verifySocketFile(endpoint string) error {
	fi, err := os.Lstat(endpoint)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("ipc: %s is not a unix domain socket", endpoint)
	}
	return nil
}

func cleanupStaleSocket(endpoint string) error {
	fi, statErr := os.Lstat(endpoint)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("ipc: stat existing socket: %w", statErr)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("ipc: refusing to remove non-socket path %s", endpoint)
	}
	// Check whether an existing helper is actively listening on this socket.
	testConn, dialErr := net.DialTimeout("unix", endpoint, 200*time.Millisecond)
	if dialErr == nil {
		_ = testConn.Close()
		return fmt.Errorf("ipc: helper socket %s is already in use", endpoint)
	}
	if err := os.Remove(endpoint); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ipc: remove stale socket: %w", err)
	}
	return nil
}

type unixListener struct {
	net.Listener
	endpoint  string
	ownerUID  uint32
	closeOnce sync.Once
	closeErr  error
}

func (l *unixListener) Accept() (net.Conn, error) {
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

func (l *unixListener) Close() error {
	l.closeOnce.Do(func() { l.closeErr = l.Listener.Close(); _ = os.Remove(l.endpoint) })
	return l.closeErr
}
