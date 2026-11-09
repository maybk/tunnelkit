package helperclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
	"tunnelkit/internal/privilege"
)

var (
	// ErrElevationCancelled is returned when the user cancels the OS administrator/UAC prompt.
	ErrElevationCancelled = privilege.ErrCancelled

	// ErrProtocolMismatch is returned when a running helper speaks a different IPC protocol version.
	ErrProtocolMismatch = errors.New("helper protocol version mismatch")

	// ErrHelperMissing is returned when the tunnelkit-helper binary cannot be found.
	ErrHelperMissing = errors.New("tunnelkit-helper binary not found")

	// ErrRecoveryPending means a failed launch may still own an active helper.
	ErrRecoveryPending = errors.New("helper cleanup is unconfirmed; recovery ticket retained")
)

// LaunchRequest describes how to spawn and initialize a new helper session.
type LaunchRequest struct {
	HelperPath      string
	Config          fortivpn.Config
	ProfileID       string
	ProfileName     string
	RecoveryOnly    bool
	RecoveryJournal json.RawMessage
	// OnReady persists the recovery ticket synchronously before start is sent.
	OnReady func(Ticket) error
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

	prepared, err := prepareClientCertificate(req.Config)
	if err != nil {
		return nil, Ticket{}, err
	}
	req.Config = prepared
	token, err := ipc.GenerateToken()
	if err != nil {
		return nil, Ticket{}, fmt.Errorf("generate session token: %w", err)
	}
	tokenHash := ipc.HashToken(token)

	ownerID, err := ipc.CurrentOwnerID()
	if err != nil {
		return nil, Ticket{}, err
	}

	endpoint, err := defaultEndpoint()
	if err != nil {
		return nil, Ticket{}, err
	}

	exited, err := launchElevated(ctx, helperPath, endpoint, ownerID, tokenHash)
	if err != nil {
		return nil, Ticket{}, err
	}

	conn, err := dialWithProcess(ctx, endpoint, 10*time.Second, exited)
	if err != nil {
		cleanupLaunch(endpoint, token)
		return nil, Ticket{}, fmt.Errorf("辅助组件未能就绪，请确认系统支持安装的特权组件: %w", err)
	}

	client, _, err := handshakeAndServe(ctx, conn, token)
	if err != nil {
		_ = conn.Close()
		cleanupLaunch(endpoint, token)
		return nil, Ticket{}, err
	}

	client.processExit = exited
	ticket := Ticket{
		Protocol:    ipc.ProtocolVersion,
		Endpoint:    endpoint,
		Token:       token,
		PID:         client.pid,
		ProfileID:   req.ProfileID,
		ProfileName: req.ProfileName,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if req.OnReady != nil {
		if err := req.OnReady(ticket); err != nil {
			_ = client.Close()
			_ = cleanupLaunch(endpoint, token)
			return nil, Ticket{}, err
		}
	}
	var startErr error
	if req.RecoveryOnly {
		startErr = client.RecoverNetwork(ctx, req.RecoveryJournal)
	} else {
		startErr = client.Start(ctx, req.Config)
	}
	if err := startErr; err != nil {
		_ = client.Close()
		if cleanupErr := cleanupLaunch(endpoint, token); cleanupErr != nil {
			return nil, ticket, fmt.Errorf("%w: %v", ErrRecoveryPending, cleanupErr)
		}
		return nil, Ticket{}, err
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
		if t.Protocol == 1 && ipc.EndpointUnavailable(err) {
			waitCtx, waitCancel := context.WithTimeout(ctx, 12*time.Second)
			defer waitCancel()
			if waitErr := (&Client{pid: t.PID}).WaitForExit(waitCtx); waitErr != nil {
				return nil, nil, waitErr
			}
		}
		return nil, nil, err
	}
	protocol := t.Protocol
	if protocol == 0 {
		protocol = ipc.ProtocolVersion
	}
	if protocol < 1 || protocol > ipc.ProtocolVersion {
		_ = conn.Close()
		return nil, nil, ErrProtocolMismatch
	}
	client, snap, err := handshakeWithProtocol(ctx, conn, t.Token, protocol)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return client, snap, nil
}

func dialWithRetry(ctx context.Context, endpoint string, maxWait time.Duration) (*ipc.Conn, error) {
	return dialWithProcess(ctx, endpoint, maxWait, nil)
}

func dialWithProcess(ctx context.Context, endpoint string, maxWait time.Duration, exited <-chan error) (*ipc.Conn, error) {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		conn, err := ipc.Dial(attemptCtx, endpoint)
		cancel()
		if err == nil {
			return conn, nil
		}
		select {
		case processErr := <-exited:
			if processErr == nil {
				processErr = errors.New("辅助组件在就绪前退出")
			}
			return nil, processErr
		default:
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func installedHelperPath(ctx context.Context, source string) (string, error) {
	if source == "" {
		var ok bool
		source, ok = LocateHelper()
		if !ok {
			return "", ErrHelperMissing
		}
	}
	if err := privilege.Ensure(ctx, source); err != nil {
		return "", err
	}
	return privilege.ExecutablePath()
}

func buildServeArgs(ownerFlag, endpoint, ownerID, tokenSHA256 string) []string {
	return []string{
		"serve",
		"--endpoint", endpoint,
		ownerFlag, ownerID,
		"--token-sha256", tokenSHA256,
	}
}

// cleanupLaunch cooperatively stops an authenticated helper after cancellation.
// The helper also exits through its short hello watchdog if no socket is ready.
func cleanupLaunch(endpoint, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialWithRetry(ctx, endpoint, 2*time.Second)
	if err != nil {
		if ipc.EndpointUnavailable(err) {
			return nil
		}
		return err
	}
	defer conn.Close()
	client, _, err := handshakeAndServe(ctx, conn, token)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Stop(ctx); err != nil {
		return err
	}
	for {
		select {
		case message, ok := <-client.Events():
			if !ok {
				return nil
			}
			if message.Event != nil && message.Event.Phase == fortivpn.PhaseDisconnected {
				if message.Event.Err != nil {
					return message.Event.Err
				}
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
