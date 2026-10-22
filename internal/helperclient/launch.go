package helperclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

var (
	// ErrElevationCancelled is returned when the user cancels the OS administrator/UAC prompt.
	ErrElevationCancelled = errors.New("elevation cancelled by user")

	// ErrProtocolMismatch is returned when a running helper speaks a different IPC protocol version.
	ErrProtocolMismatch = errors.New("helper protocol version mismatch")

	// ErrHelperMissing is returned when the tunnelkit-helper binary cannot be found.
	ErrHelperMissing = errors.New("tunnelkit-helper binary not found")
)

// Ticket stores the connection metadata needed to re-attach to a running helper
// across GUI restarts (`~/.tunnelkit/run/session.json`).
type Ticket struct {
	Endpoint    string `json:"endpoint"`
	Token       string `json:"token"`
	PID         int    `json:"pid"`
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	StartedAt   string `json:"startedAt"`
}

// LaunchRequest describes how to spawn and initialize a new helper session.
type LaunchRequest struct {
	HelperPath  string
	Endpoint    string
	LogPath     string
	JournalPath string
	Config      fortivpn.Config
	ProfileID   string
	ProfileName string
}

// LocateHelper finds the tunnelkit-helper executable next to the running GUI
// binary (or in standard system/development locations).
func LocateHelper() (string, bool) {
	exeName := "tunnelkit-helper"
	if runtime.GOOS == "windows" {
		exeName = "tunnelkit-helper.exe"
	}

	var candidates []string
	if selfExe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(selfExe), exeName))
		if resolved, err := filepath.EvalSymlinks(selfExe); err == nil && resolved != selfExe {
			candidates = append(candidates, filepath.Join(filepath.Dir(resolved), exeName))
		}
	}
	if runtime.GOOS == "linux" {
		if appDir := os.Getenv("APPDIR"); appDir != "" {
			candidates = append(candidates,
				filepath.Join(appDir, "usr", "bin", exeName),
				filepath.Join(appDir, exeName),
			)
		}
	}
	if runtime.GOOS != "windows" {
		candidates = append(candidates,
			filepath.Join("/usr/local/bin", exeName),
			filepath.Join("/usr/bin", exeName),
			filepath.Join("/opt/tunnelkit", exeName),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "bin", exeName),
			filepath.Join(wd, exeName),
		)
	}

	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			continue
		}
		return p, true
	}
	if len(candidates) > 0 {
		return candidates[0], false
	}
	return exeName, false
}

// Launch elevates and starts tunnelkit-helper, completes the hello/welcome
// handshake, sends the start command, and returns the connected Client and Ticket.
func Launch(ctx context.Context, req LaunchRequest) (*Client, Ticket, error) {
	helperPath := req.HelperPath
	if helperPath == "" {
		var ok bool
		helperPath, ok = LocateHelper()
		if !ok {
			return nil, Ticket{}, fmt.Errorf("%w: %s", ErrHelperMissing, helperPath)
		}
	} else if info, err := os.Stat(helperPath); err != nil || info.IsDir() {
		return nil, Ticket{}, fmt.Errorf("%w: %s", ErrHelperMissing, helperPath)
	}

	token, err := ipc.GenerateToken()
	if err != nil {
		return nil, Ticket{}, fmt.Errorf("generate session token: %w", err)
	}
	tokenHash := ipc.HashToken(token)

	ownerID, err := ipc.CurrentOwnerID()
	if err != nil {
		return nil, Ticket{}, err
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint, err = defaultEndpoint()
		if err != nil {
			return nil, Ticket{}, err
		}
	}

	// Remove any leftover unix socket before launching a new single-session helper.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		_ = os.Remove(endpoint)
	}

	if err := launchElevated(ctx, helperPath, endpoint, ownerID, tokenHash, req.LogPath, req.JournalPath); err != nil {
		return nil, Ticket{}, err
	}

	conn, err := dialWithRetry(ctx, endpoint, 10*time.Second)
	if err != nil {
		return nil, Ticket{}, fmt.Errorf("connect to helper IPC: %w", err)
	}

	client, _, err := handshakeAndServe(ctx, conn, token)
	if err != nil {
		_ = conn.Close()
		return nil, Ticket{}, err
	}

	if err := client.Start(ctx, req.Config); err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		_ = client.Stop(stopCtx)
		stopCancel()
		_ = client.Close()
		return nil, Ticket{}, err
	}

	ticket := Ticket{
		Endpoint:    endpoint,
		Token:       token,
		PID:         client.pid,
		ProfileID:   req.ProfileID,
		ProfileName: req.ProfileName,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	return client, ticket, nil
}

// Attach connects to an already-running helper using a saved Ticket and returns
// the Client along with the helper's current state snapshot.
func Attach(ctx context.Context, t Ticket) (*Client, *fortivpn.Event, error) {
	if t.Endpoint == "" || t.Token == "" {
		return nil, nil, errors.New("incomplete session ticket")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	conn, err := ipc.Dial(dialCtx, t.Endpoint)
	if err != nil {
		return nil, nil, err
	}
	client, snap, err := handshakeAndServe(ctx, conn, t.Token)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return client, snap, nil
}

func dialWithRetry(ctx context.Context, endpoint string, maxWait time.Duration) (*ipc.Conn, error) {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		conn, err := ipc.Dial(attemptCtx, endpoint)
		cancel()
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(50 * time.Millisecond)
	}
}
