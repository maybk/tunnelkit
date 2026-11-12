package vpn

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/storage"
)

type fakeConn struct {
	eventsCh chan helperclient.Message
	mu       sync.Mutex
	closed   bool
	stopped  int
	stopErr  error
	answers  []struct {
		id     string
		value  string
		cancel bool
	}
	onStop   func(*fakeConn)
	onAnswer func(*fakeConn, string, string, bool) error
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		eventsCh: make(chan helperclient.Message, 32),
	}
}

func (c *fakeConn) Events() <-chan helperclient.Message { return c.eventsCh }

func (c *fakeConn) Answer(_ context.Context, id, value string, cancel bool) error {
	c.mu.Lock()
	c.answers = append(c.answers, struct {
		id     string
		value  string
		cancel bool
	}{id, value, cancel})
	fn := c.onAnswer
	c.mu.Unlock()
	if fn != nil {
		return fn(c, id, value, cancel)
	}
	return nil
}

func (c *fakeConn) Stop(_ context.Context) error {
	c.mu.Lock()
	c.stopped++
	err := c.stopErr
	fn := c.onStop
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if fn != nil {
		fn(c)
	} else {
		c.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnecting})
		c.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
		_ = c.Close()
	}
	return nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.eventsCh)
	}
	return nil
}

func (c *fakeConn) sendEvent(ev fortivpn.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.eventsCh <- helperclient.Message{Event: &ev}
	}
}

func (c *fakeConn) sendPrompt(p fortivpn.Prompt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.eventsCh <- helperclient.Message{Prompt: &p}
	}
}

func (c *fakeConn) stopCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

type fakeLauncher struct {
	mu       sync.Mutex
	launchFn func(ctx context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error)
	attachFn func(ctx context.Context, t helperclient.Ticket) (Conn, *fortivpn.Event, error)
}

func (l *fakeLauncher) Launch(ctx context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
	l.mu.Lock()
	fn := l.launchFn
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	conn := newFakeConn()
	conn.sendEvent(fortivpn.Event{
		Phase: fortivpn.PhaseConnected,
		Info:  &fortivpn.TunnelInfo{LocalIP: "10.212.0.10", RouteCount: 2},
	})
	return conn, helperclient.Ticket{
		Endpoint:    "/tmp/fake.sock",
		Token:       "tok",
		PID:         4242,
		ProfileID:   req.ProfileID,
		ProfileName: req.ProfileName,
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (l *fakeLauncher) Attach(ctx context.Context, t helperclient.Ticket) (Conn, *fortivpn.Event, error) {
	l.mu.Lock()
	fn := l.attachFn
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx, t)
	}
	return nil, nil, errors.New("no running helper")
}

func setupTestService(t *testing.T, l *fakeLauncher) (*Service, profile.Profile) {
	t.Helper()
	root := t.TempDir()
	store := profile.NewStore(root)
	saved, err := store.Save(profile.Input{
		Name:     "公司 VPN",
		Host:     "vpn.example.com",
		Port:     443,
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("save profile failed: %v", err)
	}
	svc := NewService(root, store, l)
	svc.readyTimeout = 200 * time.Millisecond
	t.Cleanup(svc.detachConn)
	return svc, saved
}

func TestConnectSuccessAndDisconnect(t *testing.T) {
	l := &fakeLauncher{}
	svc, prof := setupTestService(t, l)

	var mu sync.Mutex
	var seen []Status
	svc.SetListener(func(s State) {
		mu.Lock()
		if len(seen) == 0 || seen[len(seen)-1] != s.Status {
			seen = append(seen, s.Status)
		}
		mu.Unlock()
	})

	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	st := svc.State()
	if st.Status != StatusConnected || st.ProfileID != prof.ID || st.Info == nil || st.Info.LocalIP != "10.212.0.10" {
		t.Fatalf("unexpected state after connect: %+v", st)
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("expected ticket saved in session.json: %v", err)
	}

	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}
	if st := svc.State(); st.Status != StatusDisconnected || st.Error != nil {
		t.Fatalf("expected clean disconnected state, got %+v", st)
	}
	if _, err := svc.tickets.Load(); err == nil {
		t.Fatalf("ticket must be cleared after disconnect")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []Status{StatusConnecting, StatusConnected, StatusDisconnecting, StatusDisconnected}
	if len(seen) != len(want) {
		t.Fatalf("listener observed %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("seen[%d] = %s, want %s", i, seen[i], want[i])
		}
	}
}

func TestConnectUntrustedCertExtractsDigest(t *testing.T) {
	digest := strings.Repeat("c9", 32)
	l := &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			conn := newFakeConn()
			conn.sendEvent(fortivpn.Event{
				Phase: fortivpn.PhaseDisconnected,
				Err: &fortivpn.Error{
					Code:        fortivpn.CodeCertUntrusted,
					Fingerprint: digest,
				},
			})
			return conn, helperclient.Ticket{Endpoint: "/tmp/s", Token: "t", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
		},
	}
	svc, prof := setupTestService(t, l)

	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatalf("expected connect to fail on untrusted cert")
	}
	st := svc.State()
	if st.Status != StatusDisconnected || st.Error == nil || st.Error.TrustedCert != digest {
		t.Fatalf("expected disconnected state with TrustedCert=%q, got %+v", digest, st)
	}
}

func TestConnectPromptAndAnswer(t *testing.T) {
	conn := newFakeConn()
	conn.onAnswer = func(c *fakeConn, id, value string, cancel bool) error {
		if id == "otp-1" && value == "123456" && !cancel {
			c.sendEvent(fortivpn.Event{
				Phase: fortivpn.PhaseConnected,
				Info:  &fortivpn.TunnelInfo{LocalIP: "10.200.1.5"},
			})
		}
		return nil
	}

	l := &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			conn.sendPrompt(fortivpn.Prompt{
				ID:      "otp-1",
				Kind:    fortivpn.PromptOTP,
				Message: "Enter OTP",
			})
			return conn, helperclient.Ticket{Endpoint: "/tmp/s", Token: "t", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
		},
	}
	svc, prof := setupTestService(t, l)
	svc.readyTimeout = 2 * time.Second

	promptSeen := make(chan fortivpn.Prompt, 1)
	svc.SetListener(func(s State) {
		if s.Phase == fortivpn.PhaseWaitingInput && s.Prompt != nil {
			select {
			case promptSeen <- *s.Prompt:
			default:
			}
		}
	})

	done := make(chan error, 1)
	go func() {
		done <- svc.Connect(context.Background(), prof.ID)
	}()

	select {
	case p := <-promptSeen:
		if err := svc.Answer(context.Background(), p.ID, "123456", false); err != nil {
			t.Fatalf("Answer: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Prompt state")
	}

	if err := <-done; err != nil {
		t.Fatalf("Connect failed after OTP answer: %v", err)
	}
	if st := svc.State(); st.Status != StatusConnected || st.Prompt != nil {
		t.Fatalf("expected connected state with cleared prompt, got %+v", st)
	}
}

func TestConnectAuthCancelledIsSilent(t *testing.T) {
	l := &fakeLauncher{
		launchFn: func(context.Context, helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			return nil, helperclient.Ticket{}, helperclient.ErrElevationCancelled
		},
	}
	svc, prof := setupTestService(t, l)

	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatalf("elevation cancel must not return error, got %v", err)
	}
	if st := svc.State(); st.Status != StatusDisconnected || st.Error != nil {
		t.Fatalf("expected clean disconnected state, got %+v", st)
	}
}

func TestConnectTimeoutStopsHelper(t *testing.T) {
	conn := newFakeConn()
	l := &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			return conn, helperclient.Ticket{Endpoint: "/tmp/s", Token: "t", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
		},
	}
	svc, prof := setupTestService(t, l)
	svc.readyTimeout = 40 * time.Millisecond

	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatalf("expected timeout error")
	}
	if st := svc.State(); st.Error == nil || st.Error.Message != "连接超时" {
		t.Fatalf("expected timeout error state, got %+v", st)
	}
	if conn.stopCount() != 1 {
		t.Fatalf("expected Stop called once on timeout, got %d", conn.stopCount())
	}
}

func TestRecoverAttachesAndDetectsUnexpectedHelperExit(t *testing.T) {
	conn := newFakeConn()
	l := &fakeLauncher{
		attachFn: func(_ context.Context, _ helperclient.Ticket) (Conn, *fortivpn.Event, error) {
			return conn, &fortivpn.Event{
				Phase: fortivpn.PhaseConnected,
				Info:  &fortivpn.TunnelInfo{LocalIP: "10.212.0.10"},
			}, nil
		},
	}
	svc, prof := setupTestService(t, l)
	_ = svc.tickets.Save(helperclient.Ticket{
		Endpoint:    "/tmp/s",
		Token:       "t",
		PID:         9000,
		ProfileID:   prof.ID,
		ProfileName: prof.Name,
		StartedAt:   time.Now().Format(time.RFC3339),
	})

	dropped := make(chan State, 1)
	svc.SetListener(func(s State) {
		if s.Status == StatusDisconnected && s.Error != nil {
			select {
			case dropped <- s:
			default:
			}
		}
	})

	if st := svc.Recover(context.Background()); st.Status != StatusConnected {
		t.Fatalf("expected connected on valid ticket, got %+v", st)
	}

	// Simulate helper crash (events channel closes without PhaseDisconnected)
	_ = conn.Close()

	select {
	case s := <-dropped:
		if s.Error.Message != "连接已意外断开" {
			t.Fatalf("unexpected drop reason: %+v", s.Error)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("did not detect helper IPC stream close")
	}
}

func TestLogUsesFileModTimeAndRejectsInvalidID(t *testing.T) {
	svc, prof := setupTestService(t, &fakeLauncher{})
	if _, err := svc.Log("../etc/passwd"); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("expected ErrInvalidProfile, got %v", err)
	}

	view, err := svc.Log(prof.ID)
	if err != nil || view.Exists || view.UpdatedAt != "" {
		t.Fatalf("missing log must report Exists=false without timestamp, got %+v (err=%v)", view, err)
	}

	logPath := storage.LogPath(svc.rootDir, prof.ID)
	_ = os.MkdirAll(storage.LogsDir(svc.rootDir), 0o700)
	_ = os.WriteFile(logPath, []byte("password = hunter2\n"), 0o600)
	past := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	_ = os.Chtimes(logPath, past, past)

	view, err = svc.Log(prof.ID)
	if err != nil || !view.Exists || view.UpdatedAt != past.Format(time.RFC3339) || strings.Contains(view.Content, "hunter2") {
		t.Fatalf("unexpected log view: %+v (err=%v)", view, err)
	}

	if err := svc.RemoveLog(prof.ID); err != nil {
		t.Fatalf("RemoveLog failed: %v", err)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("log file must be removed")
	}
}

func TestConnectSwitchesFromActiveProfile(t *testing.T) {
	conns := make(map[string]*fakeConn)
	var mu sync.Mutex
	l := &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			c := newFakeConn()
			c.sendEvent(fortivpn.Event{
				Phase: fortivpn.PhaseConnected,
				Info:  &fortivpn.TunnelInfo{LocalIP: "10.212.0.20", RouteCount: 1},
			})
			mu.Lock()
			conns[req.ProfileID] = c
			mu.Unlock()
			return c, helperclient.Ticket{
				Endpoint:    "/tmp/fake.sock",
				Token:       "tok-" + req.ProfileID,
				PID:         1001,
				ProfileID:   req.ProfileID,
				ProfileName: req.ProfileName,
			}, nil
		},
	}
	svc, profA := setupTestService(t, l)
	profB, err := svc.profiles.Save(profile.Input{
		Name:     "测试环境 VPN",
		Host:     "test-vpn.example.com",
		Port:     443,
		Username: "bob",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("save profile B failed: %v", err)
	}

	if err := svc.Connect(context.Background(), profA.ID); err != nil {
		t.Fatalf("Connect A failed: %v", err)
	}
	if err := svc.Connect(context.Background(), profA.ID); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("expected ErrAlreadyConnected on same profile, got %v", err)
	}

	// Switching directly to profB should automatically stop profA and connect profB.
	if err := svc.Connect(context.Background(), profB.ID); err != nil {
		t.Fatalf("Switch to B failed: %v", err)
	}
	st := svc.State()
	if st.Status != StatusConnected || st.ProfileID != profB.ID || st.ProfileName != profB.Name {
		t.Fatalf("expected connected to profB, got %+v", st)
	}
	mu.Lock()
	connA := conns[profA.ID]
	mu.Unlock()
	if connA == nil || connA.stopCount() != 1 {
		t.Fatalf("expected profA helper stopped once during switch")
	}
}

func TestConnectSwitchesFromInFlightConnectingProfile(t *testing.T) {
	connA := newFakeConn()
	connB := newFakeConn()
	connB.sendEvent(fortivpn.Event{
		Phase: fortivpn.PhaseConnected,
		Info:  &fortivpn.TunnelInfo{LocalIP: "10.212.0.21"},
	})

	svc, profA := setupTestService(t, &fakeLauncher{})
	profB, err := svc.profiles.Save(profile.Input{
		Name:     "备用 VPN",
		Host:     "backup-vpn.example.com",
		Port:     443,
		Username: "bob",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("save profile B failed: %v", err)
	}

	svc.launcher = &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			if req.ProfileID == profA.ID {
				connA.sendPrompt(fortivpn.Prompt{ID: "otp-a", Kind: fortivpn.PromptOTP, Message: "OTP"})
				return connA, helperclient.Ticket{Endpoint: "/tmp/a", Token: "ta", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
			}
			return connB, helperclient.Ticket{Endpoint: "/tmp/b", Token: "tb", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
		},
	}
	svc.readyTimeout = 2 * time.Second

	aPromptReady := make(chan struct{}, 1)
	svc.SetListener(func(s State) {
		if s.ProfileID == profA.ID && s.Phase == fortivpn.PhaseWaitingInput {
			select {
			case aPromptReady <- struct{}{}:
			default:
			}
		}
	})

	aDone := make(chan error, 1)
	go func() {
		aDone <- svc.Connect(context.Background(), profA.ID)
	}()

	select {
	case <-aPromptReady:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for profA prompt")
	}

	if err := svc.Connect(context.Background(), profB.ID); err != nil {
		t.Fatalf("Switch to B while A connecting failed: %v", err)
	}
	if err := <-aDone; err != nil {
		t.Fatalf("cancelled Connect A should return nil, got %v", err)
	}
	if connA.stopCount() != 1 {
		t.Fatalf("expected connA stopped once, got %d", connA.stopCount())
	}
	if st := svc.State(); st.Status != StatusConnected || st.ProfileID != profB.ID {
		t.Fatalf("expected connected to profB, got %+v", st)
	}
}

func TestDisconnectCancelsInFlightConnect(t *testing.T) {
	conn := newFakeConn()
	svc, prof := setupTestService(t, &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			conn.sendPrompt(fortivpn.Prompt{ID: "otp-1", Kind: fortivpn.PromptOTP, Message: "OTP"})
			return conn, helperclient.Ticket{
				Endpoint:    "/tmp/a",
				Token:       "ta",
				ProfileID:   req.ProfileID,
				ProfileName: req.ProfileName,
			}, nil
		},
	})
	svc.readyTimeout = 2 * time.Second

	promptReady := make(chan struct{}, 1)
	svc.SetListener(func(s State) {
		if s.Phase == fortivpn.PhaseWaitingInput {
			select {
			case promptReady <- struct{}{}:
			default:
			}
		}
	})

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- svc.Connect(context.Background(), prof.ID)
	}()

	select {
	case <-promptReady:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt")
	}

	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect during connecting failed: %v", err)
	}
	if err := <-connectDone; err != nil {
		t.Fatalf("Connect cancelled by Disconnect must return nil, got %v", err)
	}
	if conn.stopCount() < 1 {
		t.Fatalf("expected helper Stop called at least once, got %d", conn.stopCount())
	}
	if st := svc.State(); st.Status != StatusDisconnected || st.Error != nil {
		t.Fatalf("expected clean disconnected state, got %+v", st)
	}
}

func TestShutdownWaitsForDisconnectAndRejectsNewConnections(t *testing.T) {
	conn := newFakeConn()
	stopStarted := make(chan struct{})
	finishStop := make(chan struct{})
	conn.onStop = func(c *fakeConn) {
		close(stopStarted)
		<-finishStop
		c.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
		_ = c.Close()
	}
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
		return conn, helperclient.Ticket{Endpoint: "/tmp/s", Token: "t", ProfileID: req.ProfileID}, nil
	}})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatal(err)
	}
	disconnectDone := make(chan error, 1)
	go func() { disconnectDone <- svc.Disconnect(context.Background()) }()
	<-stopStarted
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- svc.Shutdown(context.Background()) }()
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(finishStop)
	if err := <-disconnectDone; err != nil {
		t.Fatal(err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	if err := svc.Connect(context.Background(), prof.ID); !errors.Is(err, ErrClosed) {
		t.Fatalf("Connect after shutdown: %v", err)
	}
}

func TestShutdownWaitsForRecovery(t *testing.T) {
	attached := make(chan struct{})
	resume := make(chan struct{})
	conn := newFakeConn()
	svc, prof := setupTestService(t, &fakeLauncher{attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
		close(attached)
		<-resume
		return conn, &fortivpn.Event{Phase: fortivpn.PhaseConnected}, nil
	}})
	if err := svc.tickets.Save(helperclient.Ticket{Endpoint: "/tmp/s", Token: "t", ProfileID: prof.ID}); err != nil {
		t.Fatal(err)
	}
	go svc.Recover(context.Background())
	<-attached
	done := make(chan error, 1)
	go func() { done <- svc.Shutdown(context.Background()) }()
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if conn.stopCount() != 1 || svc.State().Status != StatusDisconnected {
		t.Fatalf("recovered session was not stopped")
	}
}

func TestShutdownTimeoutKeepsRecoverableSession(t *testing.T) {
	conn := newFakeConn()
	conn.onStop = func(*fakeConn) {}
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
		return conn, helperclient.Ticket{Endpoint: "/tmp/s", Token: "t", ProfileID: req.ProfileID}, nil
	}, attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
		// 终态事件可能先被消费；重接时返回已停止快照，不能落入默认的未知 helper 错误。
		return newFakeConn(), &fortivpn.Event{Phase: fortivpn.PhaseDisconnected}, nil
	}})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := svc.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("cleanup timeout lost recovery ticket: %v", err)
	}
	if svc.State().Error == nil {
		t.Fatal("cleanup failure must be visible")
	}
	conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
	_ = conn.Close()
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTemporaryIPCFailurePreservesRecoveryTicket(t *testing.T) {
	svc, prof := setupTestService(t, &fakeLauncher{attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
		return nil, nil, context.DeadlineExceeded
	}})
	ticket := helperclient.Ticket{Endpoint: "/tmp/temporary-session", Token: "synthetic-session", ProfileID: prof.ID}
	if err := svc.tickets.Save(ticket); err != nil {
		t.Fatal(err)
	}
	if state := svc.Recover(context.Background()); state.Error == nil {
		t.Fatal("temporary attachment failure must remain visible")
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("lost recovery ticket: %v", err)
	}
	if err := svc.Disconnect(context.Background()); err == nil {
		t.Fatal("unconfirmed disconnect must return its IPC error")
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("disconnect lost recovery ticket: %v", err)
	}
	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatal("must confirm the prior session before launching another")
	}
}

type processAwareConn struct {
	*fakeConn
	waiting, exited chan struct{}
}

func (c *processAwareConn) WaitForExit(ctx context.Context) error {
	close(c.waiting)
	select {
	case <-c.exited:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestDisconnectWaitsForHelperProcessExit(t *testing.T) {
	conn := &processAwareConn{fakeConn: newFakeConn(), waiting: make(chan struct{}), exited: make(chan struct{})}
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
		return conn, helperclient.Ticket{Endpoint: "/tmp/process-test", Token: "synthetic-session", ProfileID: req.ProfileID}, nil
	}})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- svc.Disconnect(context.Background()) }()
	select {
	case <-conn.waiting:
	case <-time.After(time.Second):
		t.Fatal("missing process-exit confirmation")
	}
	select {
	case err := <-done:
		t.Fatalf("stop acknowledgement bypassed process wait: %v", err)
	default:
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("ticket must survive until confirmed process exit: %v", err)
	}
	close(conn.exited)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestFailedNetworkCleanupIsRetriedBeforeQuit(t *testing.T) {
	active := newFakeConn()
	active.onStop = func(c *fakeConn) {
		c.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: fortivpn.NewError(fortivpn.CodeNetcfgFailed, "isolated network cleanup failure")})
		_ = c.Close()
	}
	launches := 0
	svc, prof := setupTestService(t, &fakeLauncher{
		attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
			return nil, nil, os.ErrNotExist
		},
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			launches++
			conn := active
			if launches == 1 {
				conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
			} else {
				if !req.RecoveryOnly || req.Config.Host != "" {
					t.Fatal("cleanup retry must not establish a VPN connection")
				}
				conn = newFakeConn()
				conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})
			}
			return conn, helperclient.Ticket{Endpoint: "/tmp/cleanup-retry", Token: "synthetic-session", ProfileID: req.ProfileID}, nil
		},
	})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disconnect(context.Background()); err == nil {
		t.Fatal("cleanup failure must prevent successful disconnect")
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("cleanup retry information was lost: %v", err)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if launches != 2 {
		t.Fatalf("quit did not retry network recovery: %d launches", launches)
	}
	if _, err := svc.tickets.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful recovery retained ticket: %v", err)
	}
}

func TestConnectPreservesUnreadableRecoveryTicket(t *testing.T) {
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(context.Context, helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		t.Fatal("unreadable recovery information must block another launch")
		return nil, helperclient.Ticket{}, nil
	}})
	if err := storage.WriteFile(storage.TicketPath(svc.rootDir), []byte("incomplete ticket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatal("missing ticket read error")
	}
}

func TestDisconnectWithoutTerminalEventRetainsTicket(t *testing.T) {
	conn := newFakeConn()
	conn.onStop = func(c *fakeConn) { _ = c.Close() }
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
		return conn, helperclient.Ticket{Endpoint: "/tmp/stop-eof", Token: "synthetic-session", ProfileID: req.ProfileID}, nil
	}})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disconnect(context.Background()); err == nil {
		t.Fatal("IPC EOF must not prove successful network cleanup")
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("lost recovery information: %v", err)
	}
}

func TestUpdateMaintenanceGate(t *testing.T) {
	conn := newFakeConn()
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
		return conn, helperclient.Ticket{Endpoint: "/tmp/maint", Token: "tok", ProfileID: req.ProfileID}, nil
	}})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.BeginUpdate(context.Background()); err != nil {
		t.Fatalf("BeginUpdate failed: %v", err)
	}
	if err := svc.Connect(context.Background(), prof.ID); !errors.Is(err, ErrMaintenance) {
		t.Fatalf("Connect during maintenance = %v, want ErrMaintenance", err)
	}
	svc.EndUpdate(false)
	if err := svc.BeginUpdate(context.Background()); err != nil {
		t.Fatalf("second BeginUpdate failed: %v", err)
	}
	svc.EndUpdate(true)
	if err := svc.Connect(context.Background(), prof.ID); !errors.Is(err, ErrClosed) {
		t.Fatalf("Connect after restarting EndUpdate = %v, want ErrClosed", err)
	}
}

func TestFailedStopAllowsRetryViaSavedTicket(t *testing.T) {
	initialConn := newFakeConn()
	initialConn.stopErr = errors.New("ipc stop write failed")
	retryConn := newFakeConn()

	svc, prof := setupTestService(t, &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			initialConn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
			return initialConn, helperclient.Ticket{Endpoint: "/tmp/stop-retry", Token: "tok", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
		},
		attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
			return retryConn, &fortivpn.Event{Phase: fortivpn.PhaseConnected}, nil
		},
	})
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if err := svc.Disconnect(context.Background()); err == nil {
		t.Fatal("expected initial Disconnect to fail")
	}
	st := svc.State()
	if st.Status != StatusDisconnected || st.Error == nil || st.ProfileID != prof.ID {
		t.Fatalf("expected retryable disconnected error state, got %+v", st)
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatalf("failed Disconnect must retain recovery ticket: %v", err)
	}
	if err := svc.Disconnect(context.Background()); err != nil {
		t.Fatalf("retry Disconnect failed: %v", err)
	}
	if st := svc.State(); st.Status != StatusDisconnected || st.Error != nil {
		t.Fatalf("expected clean disconnected state after retry, got %+v", st)
	}
	if _, err := svc.tickets.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ticket cleared after retry Disconnect, got %v", err)
	}
}

func TestSyncSavedProfileClearsStaleConnectErrorAndPreservesRecoveryError(t *testing.T) {
	svc, prof := setupTestService(t, &fakeLauncher{
		launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			conn := newFakeConn()
			conn.sendEvent(fortivpn.Event{
				Phase: fortivpn.PhaseDisconnected,
				Err:   fortivpn.NewError(fortivpn.CodeAuthFailed, "bad password"),
			})
			return conn, helperclient.Ticket{Endpoint: "/tmp/auth-fail", Token: "tok", ProfileID: req.ProfileID, ProfileName: req.ProfileName}, nil
		},
	})
	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatal("expected auth failure")
	}
	if st := svc.State(); st.Error == nil {
		t.Fatal("expected auth error on disconnected state")
	}
	updated := prof
	updated.Name = "公司 VPN（新）"
	svc.SyncSavedProfile(updated)
	if st := svc.State(); st.Error != nil || st.ProfileName != updated.Name {
		t.Fatalf("expected stale connect error cleared and name synced, got %+v", st)
	}

	// When a recovery ticket exists, SyncSavedProfile must preserve the recovery error.
	if err := svc.tickets.Save(helperclient.Ticket{
		Endpoint:    "/tmp/pending-recovery",
		Token:       "tok",
		ProfileID:   prof.ID,
		ProfileName: updated.Name,
	}); err != nil {
		t.Fatal(err)
	}
	svc.failDisconnect(prof.ID, updated.Name, errDisconnectStop())
	svc.SyncSavedProfile(updated)
	if st := svc.State(); st.Error == nil {
		t.Fatal("recovery error must not be cleared while session ticket exists")
	}
}

func TestConnectPreLaunchFailureWritesRedactedDiagnosticLog(t *testing.T) {
	l := &fakeLauncher{
		launchFn: func(_ context.Context, _ helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			return nil, helperclient.Ticket{}, errors.New("辅助组件授权配置未完成（服务安全访问控制列表未就绪），请重新安装应用 password=mysecretpw")
		},
	}
	svc, prof := setupTestService(t, l)

	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatal("expected Connect to fail when helper launch fails")
	}
	st := svc.State()
	if st.Status != StatusDisconnected || st.Error == nil || st.Error.Message != "启动连接失败" {
		t.Fatalf("unexpected state after pre-launch failure: %+v", st)
	}

	view, err := svc.Log(prof.ID)
	if err != nil {
		t.Fatalf("Log(%s): %v", prof.ID, err)
	}
	if !view.Exists || view.UpdatedAt == "" {
		t.Fatalf("expected non-empty log snapshot after pre-launch failure, got %+v", view)
	}
	for _, want := range []string{
		"INFO  正在启动 VPN 连接",
		"ERROR 启动连接失败: 辅助组件授权配置未完成（服务安全访问控制列表未就绪），请重新安装应用",
	} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("expected log content to contain %q, got:\n%s", want, view.Content)
		}
	}
	if strings.Contains(view.Content, "mysecretpw") {
		t.Fatalf("log content leaked secret password:\n%s", view.Content)
	}
}

func TestConnectHelperMissingAndCancelWriteLifecycleLog(t *testing.T) {
	missingErr := true
	l := &fakeLauncher{
		launchFn: func(_ context.Context, _ helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			if missingErr {
				return nil, helperclient.Ticket{}, helperclient.ErrHelperMissing
			}
			return nil, helperclient.Ticket{}, helperclient.ErrElevationCancelled
		},
	}
	svc, prof := setupTestService(t, l)

	if err := svc.Connect(context.Background(), prof.ID); !errors.Is(err, helperclient.ErrHelperMissing) {
		t.Fatalf("expected ErrHelperMissing, got %v", err)
	}
	view, err := svc.Log(prof.ID)
	if err != nil || !strings.Contains(view.Content, "ERROR 未找到隧道组件 tunnelkit-helper") {
		t.Fatalf("expected helper missing error in log, got %+v (err=%v)", view, err)
	}

	missingErr = false
	if err := svc.Connect(context.Background(), prof.ID); err != nil {
		t.Fatalf("cancelled elevation must return nil, got %v", err)
	}
	view, err = svc.Log(prof.ID)
	if err != nil || !strings.Contains(view.Content, "INFO  已取消连接请求") {
		t.Fatalf("expected cancellation entry in log, got %+v (err=%v)", view, err)
	}
}
