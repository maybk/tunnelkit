package helper

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/engine"
	"tunnelkit/fortivpn/fortitest"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/ipc"
)

func TestHelperEndToEndWithOTPAndReattach(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{
		Users:      map[string]string{"alice": "password123"},
		OTPCode:    map[string]string{"alice": "888999"},
		AssignedIP: netip.MustParseAddr("10.212.0.10"),
		SplitRoutes: []netip.Prefix{
			netip.MustParsePrefix("10.10.0.0/16"),
		},
	})
	defer gw.Close()

	memDev := fortitest.NewMemDevice("utun95", 1400)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()

	token, err := ipc.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "last.log")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- ServeListener(ctx, ln, Options{
			TokenSHA256: ipc.HashToken(token),
			LogPath:     logPath,
			Version:     "0.2.0-test",
			ExitGrace:   50 * time.Millisecond,
			EngineOptions: []engine.Option{
				engine.WithDevice(memDev),
			},
		})
	}()

	dialConn := func() *ipc.Conn {
		raw, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("net.Dial: %v", err)
		}
		return ipc.NewConn(raw)
	}

	// 1. Wrong token must be rejected
	badConn := dialConn()
	_, _, err = helperclient.ConnectDial(ctx, badConn, "wrong-token")
	_ = badConn.Close()
	if err == nil {
		t.Fatalf("expected wrong token to be rejected")
	}

	// 2. Valid token connects and starts session
	cli1, _, err := helperclient.ConnectDial(ctx, dialConn(), token)
	if err != nil {
		t.Fatalf("ConnectDial cli1: %v", err)
	}
	defer cli1.Close()

	cfg := fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Auth:         fortivpn.AuthPassword,
		Username:     "alice",
		Password:     fortivpn.NewSecret("password123"),
		TrustedCerts: []string{gw.CertFingerprint()},
	}
	if err := cli1.Start(ctx, cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 3. Wait for OTP Prompt and answer it
	var sawConnected bool
	for !sawConnected {
		select {
		case msg, ok := <-cli1.Events():
			if !ok {
				t.Fatal("cli1 Events closed unexpectedly before PhaseConnected")
			}
			if msg.Prompt != nil {
				if err := cli1.Answer(ctx, msg.Prompt.ID, "888999", false); err != nil {
					t.Fatalf("Answer: %v", err)
				}
			}
			if msg.Event != nil && msg.Event.Phase == fortivpn.PhaseConnected {
				sawConnected = true
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for PhaseConnected")
		}
	}

	// 4. Simulate GUI restart: attach a second client and verify Snapshot is PhaseConnected
	cli2, snap, err := helperclient.ConnectDial(ctx, dialConn(), token)
	if err != nil {
		t.Fatalf("ConnectDial cli2: %v", err)
	}
	defer cli2.Close()

	if snap == nil || snap.Phase != fortivpn.PhaseConnected || snap.Info == nil {
		t.Fatalf("re-attached snapshot = %+v, want PhaseConnected with Info", snap)
	}
	if snap.Info.LocalIP != "10.212.0.10" {
		t.Errorf("snapshot LocalIP = %q, want 10.212.0.10", snap.Info.LocalIP)
	}

	// 5. Stop via cli2 and verify helper exits cleanly
	if err := cli2.Stop(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("ServeListener returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for helper to exit after Stop")
	}
}

func TestLoggerRedactsSensitiveValuesAndFormatsAttrs(t *testing.T) {
	var buf net.Buffers
	w := &testBufferWriter{}
	log := NewLogger(w)
	log.Error("Session failed.", "code", "auth_failed", "cookie", "secret-cookie-123", "detail", "SVPNCOOKIE=abc123xyz&credential=mysecret")
	_ = buf
	got := w.String()
	if got == "" {
		t.Fatal("expected non-empty log output")
	}
	for _, forbidden := range []string{"secret-cookie-123", "abc123xyz", "mysecret"} {
		if contains := (len(got) > 0 && stringContains(got, forbidden)); contains {
			t.Fatalf("log output leaked sensitive value %q: %s", forbidden, got)
		}
	}
	if !stringContains(got, "code=auth_failed") {
		t.Fatalf("expected structured attribute code=auth_failed in log line: %s", got)
	}
}

type testBufferWriter struct {
	b []byte
}

func (w *testBufferWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

func (w *testBufferWriter) String() string {
	return string(w.b)
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
