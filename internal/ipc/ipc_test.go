package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

type writeNotification struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (c *writeNotification) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(p)
}

func TestSendContextCancelsBlockedWriteAndResetsDeadline(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	raw := &writeNotification{Conn: left, started: make(chan struct{})}
	conn := NewConn(raw)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- conn.SendContext(ctx, TypeStop, 1, StopData{}) }()
	<-raw.started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled write: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled write remained blocked")
	}
	read := make(chan error, 1)
	go func() { _, err := NewConn(right).ReadEnvelope(); read <- err }()
	if err := conn.Send(TypeStop, 2, StopData{}); err != nil {
		t.Fatalf("previous cancellation left a write deadline: %v", err)
	}
	if err := <-read; err != nil {
		t.Fatal(err)
	}
}

func TestTokenHashAndDecodeStrict(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	hash := HashToken(tok)
	if !VerifyTokenHash(tok, hash) {
		t.Fatalf("VerifyTokenHash returned false for matching token")
	}
	if VerifyTokenHash(tok+"x", hash) {
		t.Fatalf("VerifyTokenHash returned true for wrong token")
	}

	_, err = DecodeStrict[HelloData](json.RawMessage(`{"protocol":1,"token":"abc","unknown":true}`))
	if err == nil {
		t.Fatalf("expected DecodeStrict to reject unknown field")
	}
}

func TestConnRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	left := NewConn(c1)
	right := NewConn(c2)

	errCh := make(chan error, 1)
	go func() {
		errCh <- left.Send(TypeHello, 42, HelloData{Protocol: ProtocolVersion, Token: "secret"})
	}()

	env, err := right.ReadEnvelope()
	if err != nil {
		t.Fatalf("ReadEnvelope: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Send: %v", err)
	}
	if env.Type != TypeHello || env.ID != 42 {
		t.Fatalf("got env %+v, want TypeHello ID=42", env)
	}
	hello, err := DecodeStrict[HelloData](env.Data)
	if err != nil {
		t.Fatalf("DecodeStrict: %v", err)
	}
	if hello.Token != "secret" {
		t.Errorf("hello.Token = %q, want secret", hello.Token)
	}
}

func TestUnixListenAndPeerCred(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unix socket peercred test")
	}
	ownerID, err := CurrentOwnerID()
	if err != nil {
		t.Fatalf("CurrentOwnerID: %v", err)
	}
	sockPath := filepath.Join("/tmp", "tk-ipc-test-"+mustShortToken(t)+".sock")

	ln, err := Listen(sockPath, ownerID)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()

	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		srv := NewConn(raw)
		env, err := srv.ReadEnvelope()
		if err != nil {
			return
		}
		_ = srv.Send(TypeResult, env.ID, ResultData{ID: env.ID})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cli, err := Dial(ctx, sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer cli.Close()

	if err := cli.Send(TypeStop, 7, StopData{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	reply, err := cli.ReadEnvelope()
	if err != nil {
		t.Fatalf("ReadEnvelope: %v", err)
	}
	if reply.Type != TypeResult || reply.ID != 7 {
		t.Errorf("got reply %+v, want TypeResult ID=7", reply)
	}
}

func mustShortToken(t *testing.T) string {
	t.Helper()
	tok, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	return tok[:8]
}
