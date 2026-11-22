//go:build windows

package ipc

import (
	"context"
	"testing"
	"time"
)

func TestWindowsNamedPipeListenDialAndOwnerValidation(t *testing.T) {
	if _, err := Listen(`\\.\pipe\tunnelkit-invalid-sid-test`, "not-a-sid"); err == nil {
		t.Fatal("expected Listen to reject malformed owner SID")
	}

	ownerSID, err := CurrentOwnerID()
	if err != nil {
		t.Fatalf("CurrentOwnerID: %v", err)
	}
	pipePath := `\\.\pipe\TunnelKit-test-` + mustShortToken(t)
	ln, err := Listen(pipePath, ownerSID)
	if err != nil {
		t.Fatalf("Listen(%s): %v", pipePath, err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		srv := NewConn(raw)
		env, err := srv.ReadEnvelope()
		if err == nil {
			_ = srv.Send(TypeResult, env.ID, ResultData{ID: env.ID})
		}
		_ = srv.Close()
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	cli, err := Dial(ctx, pipePath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := cli.Send(TypeStop, 9, StopData{}); err != nil {
		_ = cli.Close()
		t.Fatalf("Send: %v", err)
	}
	reply, err := cli.ReadEnvelope()
	_ = cli.Close()
	if err != nil || reply.Type != TypeResult || reply.ID != 9 {
		t.Fatalf("named pipe roundtrip got %+v err=%v, want TypeResult ID=9", reply, err)
	}
	_ = ln.Close()
	<-done
}
