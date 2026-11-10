// Package helperclient launches or attaches to `tunnelkit-helper` and provides
// a typed client for starting/stopping VPN sessions and streaming events/prompts.
package helperclient

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

// Message represents an asynchronous notification from the helper.
type Message struct {
	Event   *fortivpn.Event
	Prompt  *fortivpn.Prompt
	LogLine string
}

// Client is an authenticated IPC client connected to a running tunnelkit-helper.
type Client struct {
	conn        *ipc.Conn
	pid         int
	version     string
	protocol    int
	processExit <-chan error
	eventsCh    chan Message
	eventsMu    sync.Mutex

	seq       atomic.Uint64
	mu        sync.Mutex
	pending   map[uint64]chan ipc.ResultData
	closed    bool
	closeOnce sync.Once
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
	if c.protocol != ipc.ProtocolVersion {
		return ErrProtocolMismatch
	}
	return c.call(ctx, ipc.TypeStart, ipc.StartData{Config: cfg})
}

// RecoverNetwork restores pending host network changes without connecting to a gateway.
func (c *Client) RecoverNetwork(ctx context.Context, journal json.RawMessage) error {
	if c.protocol != ipc.ProtocolVersion {
		return ErrProtocolMismatch
	}
	return c.call(ctx, ipc.TypeRecover, ipc.RecoverData{Journal: journal})
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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

	if err := c.conn.SendContext(ctx, msgType, id, payload); err != nil {
		// A partially written frame makes the stream unusable; retain recovery data.
		_ = c.Close()
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
			if err != nil || !c.pushMessage(Message{Event: &ev}) {
				return
			}
		case ipc.TypePrompt:
			p, err := ipc.DecodeStrict[fortivpn.Prompt](env.Data)
			if err != nil || !c.pushMessage(Message{Prompt: &p}) {
				return
			}
		case ipc.TypeLog:
			entry, err := ipc.DecodeStrict[ipc.LogData](env.Data)
			if err == nil && len(entry.Line) <= 16<<10 {
				c.pushMessage(Message{LogLine: entry.Line})
			}
		case ipc.TypeResult:
			res, err := ipc.DecodeStrict[ipc.ResultData](env.Data)
			if err == nil {
				id := env.ID
				if id == 0 {
					id = res.ID
				}
				c.mu.Lock()
				if ch, ok := c.pending[id]; ok {
					select {
					case ch <- res:
					default:
					}
				}
				c.mu.Unlock()
			}
		}
	}
}
