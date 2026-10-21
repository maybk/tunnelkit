//go:build windows

package ipc

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// CurrentOwnerID returns the current process user's Windows SID string.
func CurrentOwnerID() (string, error) {
	tok := windows.GetCurrentProcessToken()
	tu, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("ipc: get current process token user: %w", err)
	}
	return tu.User.Sid.String(), nil
}

// Dial connects to the helper's Windows Named Pipe.
func Dial(ctx context.Context, endpoint string) (*Conn, error) {
	raw, err := winio.DialPipeContext(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return NewConn(raw), nil
}

// Listen creates a Windows Named Pipe with a strict SDDL DACL granting access
// only to LocalSystem (SY), Administrators (BA), and ownerSID.
func Listen(endpoint string, ownerSID string) (net.Listener, error) {
	sidStr := strings.TrimSpace(ownerSID)
	if _, err := windows.StringToSid(sidStr); err != nil {
		return nil, fmt.Errorf("ipc: invalid owner SID %q: %w", ownerSID, err)
	}

	sddl := fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;%s)", sidStr)
	cfg := &winio.PipeConfig{
		SecurityDescriptor: sddl,
		MessageMode:        false,
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}
	ln, err := winio.ListenPipe(endpoint, cfg)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen named pipe %s: %w", endpoint, err)
	}
	return ln, nil
}
