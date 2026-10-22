// Package helperclient launches or attaches to `tunnelkit-helper` and provides
// a typed client for starting/stopping VPN sessions and streaming events/prompts.
package helperclient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

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

// ConnectDial connects to an arbitrary ipc.Conn (used in unit tests without OS elevation).
func ConnectDial(ctx context.Context, conn *ipc.Conn, token string) (*Client, *fortivpn.Event, error) {
	return handshakeAndServe(ctx, conn, token)
}

func handshakeAndServe(_ context.Context, conn *ipc.Conn, token string) (*Client, *fortivpn.Event, error) {
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
