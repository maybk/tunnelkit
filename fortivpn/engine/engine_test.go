package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/fortitest"
)

type testHandler struct {
	mu          sync.Mutex
	events      []fortivpn.Event
	connectedCh chan fortivpn.TunnelInfo
	reconnCh    chan struct{}
	statsCh     chan fortivpn.Stats
	promptFn    func(ctx context.Context, p fortivpn.Prompt) (string, error)
}

func newTestHandler(promptFn func(ctx context.Context, p fortivpn.Prompt) (string, error)) *testHandler {
	return &testHandler{
		connectedCh: make(chan fortivpn.TunnelInfo, 8),
		reconnCh:    make(chan struct{}, 8),
		statsCh:     make(chan fortivpn.Stats, 16),
		promptFn:    promptFn,
	}
}

func (h *testHandler) OnEvent(ev fortivpn.Event) {
	h.mu.Lock()
	h.events = append(h.events, ev)
	h.mu.Unlock()

	if ev.Phase == fortivpn.PhaseConnected && ev.Info != nil {
		select {
		case h.connectedCh <- *ev.Info:
		default:
		}
	}
	if ev.Phase == fortivpn.PhaseConnected && ev.Stats != nil {
		select {
		case h.statsCh <- *ev.Stats:
		default:
		}
	}
	if ev.Phase == fortivpn.PhaseReconnecting {
		select {
		case h.reconnCh <- struct{}{}:
		default:
		}
	}
}

func (h *testHandler) Prompt(ctx context.Context, p fortivpn.Prompt) (string, error) {
	if h.promptFn != nil {
		return h.promptFn(ctx, p)
	}
	return "", context.Canceled
}

func (h *testHandler) Phases() []fortivpn.Phase {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []fortivpn.Phase
	for _, ev := range h.events {
		if len(out) == 0 || out[len(out)-1] != ev.Phase {
			out = append(out, ev.Phase)
		}
	}
	return out
}

func buildICMPv4EchoRequest(src, dst netip.Addr, seq uint16) []byte {
	pkt := make([]byte, 28)
	pkt[0] = 0x45 // IPv4, IHL=5 (20 bytes)
	binary.BigEndian.PutUint16(pkt[2:4], 28)
	pkt[8] = 64 // TTL
	pkt[9] = 1  // Protocol = ICMP
	s4 := src.As4()
	d4 := dst.As4()
	copy(pkt[12:16], s4[:])
	copy(pkt[16:20], d4[:])

	// ICMP Echo Request (type 8, code 0)
	pkt[20] = 8
	pkt[21] = 0
	binary.BigEndian.PutUint16(pkt[24:26], 0x1234)
	binary.BigEndian.PutUint16(pkt[26:28], seq)
	return pkt
}

func TestEngineCheck(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{
		Users:      map[string]string{"alice": "password123"},
		AssignedIP: netip.MustParseAddr("10.212.0.10"),
		DNS:        []netip.Addr{netip.MustParseAddr("10.0.0.53")},
		IPCPDNS:    []netip.Addr{netip.MustParseAddr("10.0.0.53")},
		DNSDomains: []string{"corp.example.com"},
		SplitRoutes: []netip.Prefix{
			netip.MustParsePrefix("10.10.0.0/16"),
		},
		DuplicateRoutes: []netip.Prefix{
			netip.MustParsePrefix("10.10.0.0/16"),
		},
	})
	defer gw.Close()

	cfg := fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Auth:         fortivpn.AuthPassword,
		Username:     "alice",
		Password:     fortivpn.NewSecret("password123"),
		TrustedCerts: []string{gw.CertFingerprint()},
		DNS:          fortivpn.DNSSplit,
		Routes:       fortivpn.RouteGateway,
	}

	h := newTestHandler(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := Check(ctx, cfg, h)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if info.LocalIP != "10.212.0.10" {
		t.Errorf("LocalIP = %q, want 10.212.0.10", info.LocalIP)
	}
	if info.RouteCount != 1 || len(info.Routes) != 1 || info.Routes[0] != "10.10.0.0/16" {
		t.Errorf("Routes = %v (count=%d), want [10.10.0.0/16]", info.Routes, info.RouteCount)
	}
	if gw.LogoutCount() != 1 {
		t.Errorf("LogoutCount = %d, want 1", gw.LogoutCount())
	}
}

func TestEngineRunDataPlaneAndTeardown(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{
		Users:      map[string]string{"alice": "password123"},
		AssignedIP: netip.MustParseAddr("10.212.0.10"),
		DNS:        []netip.Addr{netip.MustParseAddr("10.0.0.53")},
		IPCPDNS:    []netip.Addr{netip.MustParseAddr("10.0.0.53")},
		SplitRoutes: []netip.Prefix{
			netip.MustParsePrefix("10.10.0.0/16"),
		},
	})
	defer gw.Close()

	memDev := fortitest.NewMemDevice("utun99", 1400)
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Auth:         fortivpn.AuthPassword,
		Username:     "alice",
		Password:     fortivpn.NewSecret("password123"),
		TrustedCerts: []string{gw.CertFingerprint()},
		DNS:          fortivpn.DNSSplit,
		Routes:       fortivpn.RouteGateway,
	}

	h := newTestHandler(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(
			ctx,
			cfg,
			h,
			WithDevice(memDev),
			WithLogger(logger),
			WithEchoPolicy(40*time.Millisecond, 500*time.Millisecond, 3),
			WithStatsInterval(50*time.Millisecond),
		)
	}()

	select {
	case info := <-h.connectedCh:
		if !info.SplitDNSNeedsDomains {
			t.Errorf("expected SplitDNSNeedsDomains=true when gateway/user domains are empty")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for PhaseConnected")
	}

	// Send IPv4 ICMP Echo Request and verify reflected Echo Reply
	src := netip.MustParseAddr("10.212.0.10")
	dst := netip.MustParseAddr("10.10.1.1")
	reqPkt := buildICMPv4EchoRequest(src, dst, 1)
	if err := memDev.SendFromHost(reqPkt); err != nil {
		t.Fatalf("SendFromHost: %v", err)
	}

	recvCtx, recvCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer recvCancel()
	replyPkt, err := memDev.ReceiveOnHost(recvCtx)
	if err != nil {
		t.Fatalf("ReceiveOnHost: %v", err)
	}
	if len(replyPkt) != len(reqPkt) {
		t.Fatalf("reply length = %d, want %d", len(replyPkt), len(reqPkt))
	}
	gotSrc, _ := netip.AddrFromSlice(replyPkt[12:16])
	gotDst, _ := netip.AddrFromSlice(replyPkt[16:20])
	if gotSrc != dst || gotDst != src {
		t.Errorf("reflected packet addresses = %s -> %s, want %s -> %s", gotSrc, gotDst, dst, src)
	}
	if replyPkt[20] != 0 { // ICMP Echo Reply
		t.Errorf("ICMP type = %d, want 0 (Echo Reply)", replyPkt[20])
	}

	// Wait for Stats event with traffic and RTT counters
	select {
	case st := <-h.statsCh:
		if st.RxPackets < 1 || st.TxPackets < 1 {
			// Wait one more tick if stats fired before packet reflection completed
			st = <-h.statsCh
		}
		if st.RxPackets < 1 || st.TxPackets < 1 {
			t.Errorf("Stats packets = rx:%d tx:%d, want >= 1", st.RxPackets, st.TxPackets)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Stats event")
	}

	// Cancel context to trigger graceful disconnect
	cancel()

	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("Run returned unexpected error on graceful cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to exit")
	}

	if gw.LogoutCount() != 1 {
		t.Errorf("LogoutCount = %d, want 1", gw.LogoutCount())
	}

	logs := logBuf.String()
	for _, wantLog := range []string{
		"Connected to gateway.",
		"Authenticated.",
		"Remote gateway has allocated a VPN.",
		"Got addresses: [10.212.0.10]",
		"Negotiation complete.",
		"Interface utun99 is UP.",
		"Tunnel is up and running.",
		"Cancelling threads...",
		"Logged out.",
	} {
		if !strings.Contains(logs, wantLog) {
			t.Errorf("expected log line %q in logs:\n%s", wantLog, logs)
		}
	}
}

func TestEngineOTPAndReconnect(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{
		Users:      map[string]string{"alice": "password123"},
		OTPCode:    map[string]string{"alice": "654321"},
		AssignedIP: netip.MustParseAddr("10.212.0.10"),
		SplitRoutes: []netip.Prefix{
			netip.MustParsePrefix("10.10.0.0/16"),
		},
	})
	defer gw.Close()

	memDev := fortitest.NewMemDevice("utun98", 1400)
	var promptCalls int
	var promptMu sync.Mutex

	h := newTestHandler(func(_ context.Context, p fortivpn.Prompt) (string, error) {
		promptMu.Lock()
		promptCalls++
		promptMu.Unlock()
		if p.Kind != fortivpn.PromptOTP {
			t.Errorf("Prompt.Kind = %q, want %q", p.Kind, fortivpn.PromptOTP)
		}
		return "654321", nil
	})

	cfg := fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Auth:         fortivpn.AuthPassword,
		Username:     "alice",
		Password:     fortivpn.NewSecret("password123"),
		TrustedCerts: []string{gw.CertFingerprint()},
		Reconnect: fortivpn.Reconnect{
			Enabled:     true,
			MaxAttempts: 5,
			Delay:       40 * time.Millisecond,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(ctx, cfg, h, WithDevice(memDev))
	}()

	// 1. Initial connection (with OTP prompt)
	select {
	case <-h.connectedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for initial connection")
	}

	// 2. Drop tunnel without invalidating cookie -> should reconnect reusing cookie (no new OTP prompt)
	gw.DropActiveTunnels()
	select {
	case <-h.reconnCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PhaseReconnecting")
	}
	select {
	case <-h.connectedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reconnected state")
	}

	promptMu.Lock()
	callsAfterCookieReuse := promptCalls
	promptMu.Unlock()
	if callsAfterCookieReuse != 1 {
		t.Errorf("promptCalls after cookie reuse = %d, want 1", callsAfterCookieReuse)
	}

	// 3. Invalidate cookie AND drop tunnel -> should re-authenticate and prompt for OTP again
	gw.InvalidateCookies()
	gw.DropActiveTunnels()
	select {
	case <-h.connectedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reconnected state after cookie invalidation")
	}

	promptMu.Lock()
	callsAfterReauth := promptCalls
	promptMu.Unlock()
	if callsAfterReauth != 2 {
		t.Errorf("promptCalls after cookie invalidation = %d, want 2", callsAfterReauth)
	}

	cancel()
	if err := <-runErrCh; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestEngineEchoDeadDetection(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{
		Users:      map[string]string{"alice": "password123"},
		AssignedIP: netip.MustParseAddr("10.212.0.10"),
	})
	defer gw.Close()

	memDev := fortitest.NewMemDevice("utun97", 1400)
	h := newTestHandler(nil)

	cfg := fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Auth:         fortivpn.AuthPassword,
		Username:     "alice",
		Password:     fortivpn.NewSecret("password123"),
		TrustedCerts: []string{gw.CertFingerprint()},
		Reconnect: fortivpn.Reconnect{
			Enabled: false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(
			ctx,
			cfg,
			h,
			WithDevice(memDev),
			WithEchoPolicy(25*time.Millisecond, 20*time.Millisecond, 2),
		)
	}()

	select {
	case <-h.connectedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for PhaseConnected")
	}

	// Make gateway drop LCP Echo requests
	gw.SetDropEcho(true)

	select {
	case err := <-runErrCh:
		if !fortivpn.IsCode(err, fortivpn.CodePeerDead) {
			t.Fatalf("Run error = %v, want CodePeerDead", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for peer_dead failure")
	}
}
