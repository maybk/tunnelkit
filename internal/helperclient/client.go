// Package helperclient launches or attaches to `tunnelkit-helper` and provides
// a typed client for starting/stopping VPN sessions and streaming events/prompts.
package helperclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
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

// Message represents an asynchronous notification from the helper.
type Message struct {
	Event  *fortivpn.Event
	Prompt *fortivpn.Prompt
}

// Client is an authenticated IPC client connected to a running tunnelkit-helper.
type Client struct {
	conn     *ipc.Conn
	pid      int
	version  string
	eventsCh chan Message

	seq       atomic.Uint64
	mu        sync.Mutex
	pending   map[uint64]chan ipc.ResultData
	closed    bool
	closeOnce sync.Once
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

// ConnectDial connects to an arbitrary ipc.Conn (used in unit tests without OS elevation).
func ConnectDial(ctx context.Context, conn *ipc.Conn, token string) (*Client, *fortivpn.Event, error) {
	return handshakeAndServe(ctx, conn, token)
}

func handshakeAndServe(ctx context.Context, conn *ipc.Conn, token string) (*Client, *fortivpn.Event, error) {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := conn.Send(ipc.TypeHello, 1, ipc.HelloData{
		Protocol: ipc.ProtocolVersion,
		Token:    token,
	}); err != nil {
		return nil, nil, err
	}

	env, err := conn.ReadEnvelope()
	_ = conn.SetDeadline(time.Time{})
	if err != nil {
		return nil, nil, err
	}

	if env.Type == ipc.TypeResult {
		res, _ := ipc.DecodeStrict[ipc.ResultData](env.Data)
		if res.Error == "protocol_mismatch" {
			return nil, nil, ErrProtocolMismatch
		}
		return nil, nil, fmt.Errorf("helper rejected hello: %s", res.Error)
	}
	if env.Type != ipc.TypeWelcome {
		return nil, nil, fmt.Errorf("unexpected handshake response %q", env.Type)
	}

	welcome, err := ipc.DecodeStrict[ipc.WelcomeData](env.Data)
	if err != nil {
		return nil, nil, err
	}
	if welcome.Protocol != ipc.ProtocolVersion {
		return nil, nil, ErrProtocolMismatch
	}

	c := &Client{
		conn:     conn,
		pid:      welcome.PID,
		version:  welcome.Version,
		eventsCh: make(chan Message, 64),
		pending:  make(map[uint64]chan ipc.ResultData),
	}
	go c.readLoop()
	return c, welcome.Snapshot, nil
}

// PID returns the helper process ID reported during welcome.
func (c *Client) PID() int { return c.pid }

// Events returns the channel of asynchronous Event and Prompt messages.
// The channel is closed when the IPC connection terminates.
func (c *Client) Events() <-chan Message {
	return c.eventsCh
}

// Start sends the start command with cfg to the helper.
func (c *Client) Start(ctx context.Context, cfg fortivpn.Config) error {
	return c.call(ctx, ipc.TypeStart, ipc.StartData{Config: cfg})
}

// Answer submits the user's OTP/FTM/SAML response (or cancellation) to the helper.
func (c *Client) Answer(ctx context.Context, promptID, value string, cancel bool) error {
	return c.call(ctx, ipc.TypeAnswer, ipc.AnswerData{
		PromptID: promptID,
		Value:    value,
		Cancel:   cancel,
	})
}

// Stop requests a graceful VPN disconnect from the helper.
func (c *Client) Stop(ctx context.Context) error {
	return c.call(ctx, ipc.TypeStop, ipc.StopData{})
}

// Close closes the IPC connection to the helper without stopping the VPN session.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
		_ = c.conn.Close()
	})
	return nil
}

func (c *Client) call(ctx context.Context, msgType ipc.MessageType, payload any) error {
	id := c.seq.Add(1) + 1
	ch := make(chan ipc.ResultData, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("helper IPC connection is closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.conn.Send(msgType, id, payload); err != nil {
		return err
	}

	select {
	case res, ok := <-ch:
		if !ok {
			return errors.New("helper IPC connection closed while waiting for response")
		}
		if res.Error != "" {
			return errors.New(res.Error)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) readLoop() {
	defer func() {
		_ = c.Close()
		close(c.eventsCh)
	}()

	for {
		env, err := c.conn.ReadEnvelope()
		if err != nil {
			return
		}
		switch env.Type {
		case ipc.TypeEvent:
			ev, err := ipc.DecodeStrict[fortivpn.Event](env.Data)
			if err == nil {
				c.pushMessage(Message{Event: &ev})
			}
		case ipc.TypePrompt:
			p, err := ipc.DecodeStrict[fortivpn.Prompt](env.Data)
			if err == nil {
				c.pushMessage(Message{Prompt: &p})
			}
		case ipc.TypeResult:
			res, err := ipc.DecodeStrict[ipc.ResultData](env.Data)
			if err == nil {
				id := env.ID
				if id == 0 {
					id = res.ID
				}
				c.mu.Lock()
				ch, ok := c.pending[id]
				c.mu.Unlock()
				if ok {
					select {
					case ch <- res:
					default:
					}
				}
			}
		}
	}
}

func (c *Client) pushMessage(m Message) {
	select {
	case c.eventsCh <- m:
	default:
		// If channel is full and the incoming message is a phase change or prompt,
		// drop the oldest message to guarantee state transitions are never lost.
		if m.Prompt != nil || (m.Event != nil && m.Event.Stats == nil) {
			select {
			case <-c.eventsCh:
			default:
			}
			select {
			case c.eventsCh <- m:
			default:
			}
		}
	}
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
