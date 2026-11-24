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
