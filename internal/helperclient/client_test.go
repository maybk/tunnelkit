package helperclient

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

func TestConnectDialProtocolMismatchAndRejectedHello(t *testing.T) {
	t.Run("protocol_mismatch_result", func(t *testing.T) {
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()

		go func() {
			srv := ipc.NewConn(c2)
			_, _ = srv.ReadEnvelope()
			_ = srv.Send(ipc.TypeResult, 1, ipc.ResultData{
				ID:    1,
				Error: "protocol_mismatch",
			})
		}()

		_, _, err := ConnectDial(context.Background(), ipc.NewConn(c1), "token")
		if !errors.Is(err, ErrProtocolMismatch) {
			t.Fatalf("ConnectDial err = %v, want ErrProtocolMismatch", err)
		}
	})

	t.Run("welcome_protocol_version_mismatch", func(t *testing.T) {
		c1, c2 := net.Pipe()
		defer c1.Close()
		defer c2.Close()

		go func() {
			srv := ipc.NewConn(c2)
			_, _ = srv.ReadEnvelope()
			_ = srv.Send(ipc.TypeWelcome, 1, ipc.WelcomeData{
				Protocol: ipc.ProtocolVersion + 99,
				PID:      42,
			})
		}()

		_, _, err := ConnectDial(context.Background(), ipc.NewConn(c1), "token")
		if !errors.Is(err, ErrProtocolMismatch) {
			t.Fatalf("ConnectDial err = %v, want ErrProtocolMismatch", err)
		}
	})
}

func TestClientRPCCallsAndEventBackpressure(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	srv := ipc.NewConn(c2)
	go func() {
		_, _ = srv.ReadEnvelope()
		_ = srv.Send(ipc.TypeWelcome, 1, ipc.WelcomeData{
			Protocol: ipc.ProtocolVersion,
			PID:      9001,
			Version:  "0.1.0",
		})

		for {
			env, err := srv.ReadEnvelope()
			if err != nil {
				return
			}
			_ = srv.Send(ipc.TypeResult, env.ID, ipc.ResultData{
				ID: env.ID,
			})
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, _, err := ConnectDial(ctx, ipc.NewConn(c1), "token-ok")
	if err != nil {
		t.Fatalf("ConnectDial failed: %v", err)
	}
	defer client.Close()

	if client.PID() != 9001 {
		t.Fatalf("PID() = %d, want 9001", client.PID())
	}

	if err := client.Start(ctx, fortivpn.Config{Host: "vpn.example.com", Port: 443}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := client.Answer(ctx, "prompt-1", "123456", false); err != nil {
		t.Fatalf("Answer failed: %v", err)
	}
	if err := client.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Fill eventsCh with stats updates and verify a critical phase change is still delivered.
	for i := 0; i < 64; i++ {
		client.pushMessage(Message{
			Event: &fortivpn.Event{
				Phase: fortivpn.PhaseConnected,
				Stats: &fortivpn.Stats{RxBytes: uint64(i + 1)},
			},
		})
	}
	client.pushMessage(Message{
		Prompt: &fortivpn.Prompt{
			ID:   "otp-important",
			Kind: fortivpn.PromptOTP,
		},
	})

	var foundPrompt bool
	for i := 0; i < 64; i++ {
		msg := <-client.Events()
		if msg.Prompt != nil && msg.Prompt.ID == "otp-important" {
			foundPrompt = true
			break
		}
	}
	if !foundPrompt {
		t.Fatal("expected critical Prompt message to survive full channel backpressure")
	}
}

func TestAttachRejectsIncompleteTicket(t *testing.T) {
	if _, _, err := Attach(context.Background(), Ticket{}); err == nil {
		t.Fatal("expected Attach with empty Ticket to fail")
	}
}
