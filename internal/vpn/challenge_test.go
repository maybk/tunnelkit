package vpn

import (
	"context"
	"errors"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
)

func TestRecoveredChallengeCanBeCancelledAndReportsTransportErrors(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	p := &fortivpn.Prompt{ID: "synthetic-challenge", Kind: fortivpn.PromptOTP}
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput, Prompt: p}, helperclient.Ticket{})
	failure := errors.New("synthetic IPC failure")
	conn.stopErr = failure
	if err := svc.Answer(context.Background(), p.ID, "", true); !errors.Is(err, failure) {
		t.Fatalf("cancel must report transport failure, got %v", err)
	}
}

func TestCancelAfterAnswerAcceptedStillStopsOwningSession(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	p := &fortivpn.Prompt{ID: "accepted", Kind: fortivpn.PromptOTP}
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput, Prompt: p}, helperclient.Ticket{})
	conn.onAnswer = func(_ *fakeConn, _, _ string, cancel bool) error {
		if cancel {
			return errors.New("no active prompt")
		}
		return nil
	}
	if err := svc.Answer(context.Background(), p.ID, "synthetic-answer", false); err != nil {
		t.Fatal(err)
	}
	// 模拟 helper 已接收回答，但新的阶段事件尚未到达桌面进程。
	if err := svc.Answer(context.Background(), p.ID, "", true); err != nil {
		t.Fatalf("cancellation after answer acceptance failed: %v", err)
	}
	if conn.stopCount() != 1 {
		t.Fatal("cancel did not stop its owning session")
	}
}

func TestExpiredChallengeDoesNotCancelAnotherConnection(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseConnected}, helperclient.Ticket{})
	cancelled := false
	svc.cancelConnect = func() { cancelled = true }
	if err := svc.Answer(context.Background(), "expired-challenge", "", true); err != nil {
		t.Fatal(err)
	}
	if cancelled || len(conn.answers) != 0 || conn.stopCount() != 0 {
		t.Fatal("expired challenge affected the current connection")
	}
}

func TestConnectedSnapshotCannotRestoreChallenge(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	state := svc.adoptSession(conn, &fortivpn.Event{
		Phase:  fortivpn.PhaseConnected,
		Prompt: &fortivpn.Prompt{ID: "expired", Kind: fortivpn.PromptOTP},
	}, helperclient.Ticket{})
	if state.Status != StatusConnected || state.Prompt != nil {
		t.Fatal("connected snapshot restored a stale challenge")
	}
}

func TestRecoveredChallengeIsAnswerableDuringInitialNotification(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	svc.SetListener(func(state State) {
		if state.Prompt != nil {
			if err := svc.Answer(context.Background(), state.Prompt.ID, "synthetic-answer", false); err != nil {
				t.Errorf("initial challenge has no bound connection: %v", err)
			}
		}
	})
	svc.adoptSession(conn, &fortivpn.Event{
		Phase:  fortivpn.PhaseWaitingInput,
		Prompt: &fortivpn.Prompt{ID: "active", Kind: fortivpn.PromptOTP},
	}, helperclient.Ticket{})
}

func TestLatePromptCannotReopenConnectedSession(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseConnected}, helperclient.Ticket{})
	processed := make(chan State, 2)
	svc.SetListener(func(state State) { processed <- state })
	conn.sendPrompt(fortivpn.Prompt{ID: "late", Kind: fortivpn.PromptOTP})
	conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected, Stats: &fortivpn.Stats{RxBytes: 1}})
	select {
	case state := <-processed:
		if state.Prompt != nil || state.Status != StatusConnected {
			t.Fatal("late prompt reopened an established session")
		}
	case <-time.After(time.Second):
		t.Fatal("event stream stalled")
	}
}

func TestInputWaitDoesNotConsumeNetworkReadyTimeout(t *testing.T) {
	conn := newFakeConn()
	svc, prof := setupTestService(t, &fakeLauncher{launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		conn.sendPrompt(fortivpn.Prompt{ID: "input-wait", Kind: fortivpn.PromptOTP})
		return conn, helperclient.Ticket{ProfileID: req.ProfileID, Endpoint: "/tmp/synthetic-input", Token: "synthetic-ticket"}, nil
	}})
	svc.readyTimeout = 30 * time.Millisecond
	seen := make(chan struct{}, 1)
	svc.SetListener(func(state State) {
		if state.Prompt != nil {
			select {
			case seen <- struct{}{}:
			default:
			}
		}
	})
	done := make(chan error, 1)
	go func() { done <- svc.Connect(context.Background(), prof.ID) }()
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("no input request")
	}
	select {
	case err := <-done:
		t.Fatalf("input wait consumed network timeout: %v", err)
	case <-time.After(60 * time.Millisecond):
	}
	conn.sendEvent(fortivpn.Event{Phase: fortivpn.PhaseConnected})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCancelStopsRecoveredAuthentication(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseAuthenticating}, helperclient.Ticket{})
	if err := svc.Cancel(); err != nil {
		t.Fatal(err)
	}
	if conn.stopCount() != 1 || svc.State().Status != StatusDisconnected {
		t.Fatal("recovered authentication was not stopped")
	}
}

func TestOldCancellationCompletionCannotCancelANewerOperation(t *testing.T) {
	conn := newFakeConn()
	svc, _ := setupTestService(t, &fakeLauncher{})
	p := &fortivpn.Prompt{ID: "first", Kind: fortivpn.PromptOTP}
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput, Prompt: p}, helperclient.Ticket{ProfileID: "first"})
	oldCancelled, newCancelled := false, false
	svc.mu.Lock()
	svc.inFlightProfileID, svc.cancelConnect = "first", func() { oldCancelled = true }
	svc.mu.Unlock()
	conn.onStop = func(*fakeConn) {
		svc.mu.Lock()
		svc.cancelConnect = func() { newCancelled = true }
		svc.mu.Unlock()
	}
	if err := svc.Answer(context.Background(), p.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if !oldCancelled || newCancelled {
		t.Fatal("answer completion cancelled a different operation")
	}
}

func TestReauthenticationAcceptsNewChallengeButIgnoresOldStatistics(t *testing.T) {
	state := State{Status: StatusConnected, Phase: fortivpn.PhaseConnected}
	ticket := helperclient.Ticket{ProfileID: "synthetic"}
	for _, phase := range []fortivpn.Phase{fortivpn.PhaseReconnecting, fortivpn.PhaseAuthenticating} {
		if !applyMessage(&state, helperclient.Message{Event: &fortivpn.Event{Phase: phase}}, ticket) {
			t.Fatal("legitimate reauthentication was rejected")
		}
	}
	p := &fortivpn.Prompt{ID: "new-attempt", Kind: fortivpn.PromptOTP}
	if !applyMessage(&state, helperclient.Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput, Prompt: p}}, ticket) || state.Prompt == nil {
		t.Fatal("reauthentication prompt was lost")
	}
	if applyMessage(&state, helperclient.Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseConnected, Stats: &fortivpn.Stats{RxBytes: 1}}}, ticket) {
		t.Fatal("old statistics marked authentication complete")
	}
	if state.Prompt == nil || state.Prompt.ID != p.ID {
		t.Fatal("statistics revoked a current challenge")
	}
}

func TestSupersededStreamCannotPublishAStateRevision(t *testing.T) {
	svc, _ := setupTestService(t, &fakeLauncher{})
	oldConn, current := newFakeConn(), newFakeConn()
	defer oldConn.Close()
	defer current.Close()
	svc.mu.Lock()
	svc.conn = current
	svc.mu.Unlock()
	before := svc.State()
	if _, applied := svc.updateState(oldConn, func(state *State) bool { state.Phase = fortivpn.PhaseWaitingInput; return true }); applied {
		t.Fatal("superseded stream published a state")
	}
	if svc.State().Revision != before.Revision {
		t.Fatal("discarded state advanced revision")
	}
	state := svc.setState(State{Status: StatusDisconnected})
	if state.Revision <= before.Revision {
		t.Fatal("published state has no monotonic revision")
	}
}

func TestFailedStopDoesNotRestoreAChallengeSnapshot(t *testing.T) {
	conn := newFakeConn()
	conn.stopErr = errors.New("synthetic stop failure")
	svc, _ := setupTestService(t, &fakeLauncher{})
	svc.adoptSession(conn, &fortivpn.Event{Phase: fortivpn.PhaseWaitingInput, Prompt: &fortivpn.Prompt{ID: "active", Kind: fortivpn.PromptOTP}}, helperclient.Ticket{ProfileID: "p1"})
	if err := svc.Disconnect(context.Background()); err == nil {
		t.Fatal("stop failure was hidden")
	}
	st := svc.State()
	if st.Prompt != nil {
		t.Fatal("failed stop restored an old challenge")
	}
	if st.Status != StatusDisconnected || st.Error == nil {
		t.Fatalf("failed stop must settle into retryable disconnected error state, got %+v", st)
	}
}

func TestReconnectClearsLiveTelemetryWithoutPrematureFailure(t *testing.T) {
	state := State{
		Status:      StatusConnected,
		Phase:       fortivpn.PhaseConnected,
		ConnectedAt: "2026-11-11T00:00:00Z",
		Info:        &fortivpn.TunnelInfo{LocalIP: "10.212.0.10"},
		Stats:       &fortivpn.Stats{RxBytes: 1024},
	}
	ticket := helperclient.Ticket{ProfileID: "p1", StartedAt: "2026-11-11T00:00:00Z"}
	dropErr := fortivpn.NewError(fortivpn.CodeConnectionLost, "link lost")
	if !applyMessage(&state, helperclient.Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseReconnecting, Err: dropErr}}, ticket) {
		t.Fatal("reconnecting event was rejected")
	}
	if state.Status != StatusConnecting || state.Error != nil || state.Info != nil || state.Stats != nil || state.ConnectedAt != "" {
		t.Fatalf("reconnecting leaked stale telemetry or premature error: %+v", state)
	}
	if !applyMessage(&state, helperclient.Message{Event: &fortivpn.Event{Phase: fortivpn.PhaseDisconnected, Err: dropErr}}, ticket) {
		t.Fatal("terminal disconnect event was rejected")
	}
	if state.Status != StatusDisconnected || state.Error == nil {
		t.Fatalf("terminal disconnect lost failure reason: %+v", state)
	}
}
