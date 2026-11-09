package helperclient

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

// ConnectDial connects to an arbitrary ipc.Conn (used in unit tests without OS elevation).
func ConnectDial(ctx context.Context, conn *ipc.Conn, token string) (*Client, *fortivpn.Event, error) {
	return handshakeAndServe(ctx, conn, token)
}

func handshakeAndServe(ctx context.Context, conn *ipc.Conn, token string) (*Client, *fortivpn.Event, error) {
	return handshakeWithProtocol(ctx, conn, token, ipc.ProtocolVersion)
}

func handshakeWithProtocol(ctx context.Context, conn *ipc.Conn, token string, protocol int) (*Client, *fortivpn.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	_ = conn.SetDeadline(deadline)
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()); close(cancelled) })
	finish := sync.OnceFunc(func() {
		if !stop() {
			<-cancelled
		}
		_ = conn.SetDeadline(time.Time{})
	})
	defer finish()
	if err := conn.Send(ipc.TypeHello, 1, ipc.HelloData{
		Protocol: protocol,
		Token:    token,
	}); err != nil {
		return nil, nil, err
	}

	env, err := conn.ReadEnvelope()
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
	if welcome.Protocol != protocol {
		return nil, nil, ErrProtocolMismatch
	}

	finish()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	c := &Client{
		conn:     conn,
		pid:      welcome.PID,
		version:  welcome.Version,
		protocol: protocol,
		eventsCh: make(chan Message, 64),
		pending:  make(map[uint64]chan ipc.ResultData),
	}
	go c.readLoop()
	return c, welcome.Snapshot, nil
}
