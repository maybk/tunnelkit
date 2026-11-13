package ipc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// Conn wraps a stream net.Conn with length-prefixed JSON framing.
type Conn struct {
	raw     net.Conn
	writeMu sync.Mutex
	readMu  sync.Mutex
}

// NewConn wraps an established stream connection.
func NewConn(raw net.Conn) *Conn {
	return &Conn{raw: raw}
}

// WriteEnvelope serializes and writes an Envelope with a 4-byte big-endian length prefix.
func (c *Conn) WriteEnvelope(env Envelope) error {
	return c.writeEnvelope(context.Background(), env)
}

func (c *Conn) writeEnvelope(ctx context.Context, env Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("ipc: marshal envelope: %w", err)
	}
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("ipc: outbound frame size %d exceeds maximum %d", len(payload), MaxFrameSize)
	}

	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	copy(frame[4:], payload)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	deadline := time.Now().Add(5 * time.Second)
	usedCtxDeadline := false
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
		usedCtxDeadline = true
	}
	_ = c.raw.SetWriteDeadline(deadline)

	var (
		stop     func() bool
		finished chan struct{}
	)
	if ctx.Done() != nil {
		finished = make(chan struct{})
		stop = context.AfterFunc(ctx, func() {
			_ = c.raw.SetWriteDeadline(time.Now())
			close(finished)
		})
	}
	defer func() {
		if stop != nil && !stop() {
			<-finished
		}
		_ = c.raw.SetWriteDeadline(time.Time{})
	}()

	for len(frame) > 0 {
		n, err := c.raw.Write(frame)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if usedCtxDeadline && errors.Is(err, os.ErrDeadlineExceeded) {
				return context.DeadlineExceeded
			}
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}

// Send marshals payload into an Envelope and writes it to the connection.
func (c *Conn) Send(msgType MessageType, id uint64, payload any) error {
	env, err := NewEnvelope(msgType, id, payload)
	if err != nil {
		return err
	}
	return c.WriteEnvelope(env)
}

// SendContext bounds a complete frame write. The write lock protects deadline
// changes so cancellation cannot leave a deadline on a subsequent write.
func (c *Conn) SendContext(ctx context.Context, msgType MessageType, id uint64, payload any) error {
	env, err := NewEnvelope(msgType, id, payload)
	if err != nil {
		return err
	}
	return c.writeEnvelope(ctx, env)
}

// ReadEnvelope reads and decodes the next length-prefixed Envelope from the connection.
func (c *Conn) ReadEnvelope() (Envelope, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	var hdr [4]byte
	if _, err := io.ReadFull(c.raw, hdr[:]); err != nil {
		return Envelope{}, err
	}
	size := binary.BigEndian.Uint32(hdr[:])
	if size == 0 || size > MaxFrameSize {
		return Envelope{}, fmt.Errorf("ipc: invalid inbound frame size %d", size)
	}

	buf := make([]byte, size)
	if _, err := io.ReadFull(c.raw, buf); err != nil {
		return Envelope{}, err
	}

	var env Envelope
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return Envelope{}, fmt.Errorf("ipc: decode envelope: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return Envelope{}, errors.New("ipc: unexpected trailing envelope data")
	}
	return env, nil
}

// SetDeadline sets the read and write deadlines on the underlying connection.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.raw.SetDeadline(t)
}

// SetReadDeadline sets the read deadline on the underlying connection.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.raw.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the underlying connection.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.raw.SetWriteDeadline(t)
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	return c.raw.Close()
}
