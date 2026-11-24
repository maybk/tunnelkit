package vpn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/storage"
)

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
