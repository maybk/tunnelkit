package helperclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

type cancelOnWelcome struct {
	net.Conn
	cancel context.CancelFunc
}

func (c cancelOnWelcome) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if len(p) > 4 && n == len(p) {
		c.cancel()
	}
	return n, err
}

func TestHandshakeDoesNotTransferACancelledRequest(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		server := ipc.NewConn(right)
		_, _ = server.ReadEnvelope()
		_ = server.Send(ipc.TypeWelcome, 1, ipc.WelcomeData{Protocol: ipc.ProtocolVersion, PID: 123})
	}()
	client, _, err := ConnectDial(ctx, ipc.NewConn(cancelOnWelcome{left, cancel}), "synthetic-session")
	if client != nil {
		defer client.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("handshake returned %v, want cancelled request", err)
	}
}

func TestEarlierProtocolOnlyControlsExistingSession(t *testing.T) {
	for _, protocol := range []int{1, 2} {
		t.Run(fmt.Sprint(protocol), func(t *testing.T) {
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			go func() {
				server := ipc.NewConn(right)
				env, _ := server.ReadEnvelope()
				hello, _ := ipc.DecodeStrict[ipc.HelloData](env.Data)
				if hello.Protocol != protocol {
					return
				}
				_ = server.Send(ipc.TypeWelcome, 1, ipc.WelcomeData{Protocol: protocol, PID: 123})
				env, _ = server.ReadEnvelope()
				_ = server.Send(ipc.TypeResult, env.ID, ipc.ResultData{ID: env.ID})
			}()
			client, _, err := handshakeWithProtocol(context.Background(), ipc.NewConn(left), "synthetic-session", protocol)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.Start(context.Background(), fortivpn.Config{}); !errors.Is(err, ErrProtocolMismatch) {
				t.Fatal("earlier protocol must not start a new session")
			}
			if err := client.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
