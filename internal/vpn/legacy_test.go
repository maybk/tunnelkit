package vpn

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/ipc"
	"tunnelkit/internal/storage"
)

func TestLegacyTicketCanBeAdoptedAndDisconnected(t *testing.T) {
	conn := newFakeConn()
	launcher := &fakeLauncher{attachFn: func(_ context.Context, ticket helperclient.Ticket) (Conn, *fortivpn.Event, error) {
		if ticket.Protocol != 1 || ticket.Token != "synthetic-old-session" {
			t.Fatal("legacy handshake identity was lost")
		}
		return conn, &fortivpn.Event{Phase: fortivpn.PhaseConnected}, nil
	}}
	svc, prof := setupTestService(t, launcher)
	raw, _ := json.Marshal(map[string]any{"endpoint": "/tmp/legacy-synthetic", "token": "synthetic-old-session", "pid": 12345, "profileId": prof.ID})
	if err := storage.WriteFile(storage.TicketPath(svc.rootDir), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if state := svc.Recover(context.Background()); state.Status != StatusConnected {
		t.Fatal("legacy session was not adopted")
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if conn.stopCount() != 1 {
		t.Fatal("legacy session was not stopped")
	}
	if _, err := svc.tickets.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("clean legacy session retained its ticket")
	}
}

func TestLegacyJournalIsRetainedUntilConfirmedRecovery(t *testing.T) {
	attempts := 0
	launcher := &fakeLauncher{attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
		return nil, nil, os.ErrNotExist
	}}
	svc, _ := setupTestService(t, launcher)
	legacy := storage.LegacyJournalPath(svc.rootDir)
	if err := storage.WriteFile(legacy, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	launcher.launchFn = func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		attempts++
		if !req.RecoveryOnly || string(req.RecoveryJournal) != "[]" || req.Config.Host != "" {
			t.Fatal("legacy recovery must not connect to a gateway")
		}
		ticket := helperclient.Ticket{Endpoint: "/tmp/recovery-synthetic", Token: "synthetic-session", Protocol: ipc.ProtocolVersion}
		if err := req.OnReady(ticket); err != nil {
			return nil, ticket, err
		}
		conn := newFakeConn()
		ev := fortivpn.Event{Phase: fortivpn.PhaseDisconnected}
		if attempts == 1 {
			ev.Err = fortivpn.NewError(fortivpn.CodeNetcfgFailed, "synthetic incomplete cleanup")
		}
		conn.sendEvent(ev)
		return conn, ticket, nil
	}
	if state := svc.Recover(context.Background()); state.Error == nil {
		t.Fatal("recovery failure was not visible")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy recovery snapshot was lost")
	}
	if _, err := svc.tickets.Load(); err != nil {
		t.Fatal("recovery ticket was lost")
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("confirmed recovery did not retire legacy journal")
	}
	if attempts != 2 {
		t.Fatal("failed recovery was not retried")
	}
}

func TestAbsentLegacySessionDoesNotRequireAnotherLaunch(t *testing.T) {
	svc, _ := setupTestService(t, &fakeLauncher{attachFn: func(context.Context, helperclient.Ticket) (Conn, *fortivpn.Event, error) {
		return nil, nil, os.ErrNotExist
	}, launchFn: func(context.Context, helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
		t.Fatal("clean retired session must not request elevation")
		return nil, helperclient.Ticket{}, nil
	}})
	if err := svc.tickets.Save(helperclient.Ticket{Protocol: 1, Endpoint: "/tmp/synthetic-old", Token: "synthetic-session"}); err != nil {
		t.Fatal(err)
	}
	if state := svc.Recover(context.Background()); state.Error != nil {
		t.Fatal(state.Error)
	}
	if err := svc.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(svc.rootDir, "run", "session.json"))
	if strings.Contains(string(data), "synthetic-session") {
		t.Fatal("retired credentials were not removed")
	}
}

func TestActiveLegacySessionRecoversNetworkBeforeFinishing(t *testing.T) {
	operations := map[string]func(*Service) error{
		"disconnect": func(s *Service) error { return s.Disconnect(context.Background()) },
		"shutdown":   func(s *Service) error { return s.Shutdown(context.Background()) },
		"update":     func(s *Service) error { return s.BeginUpdate(context.Background()) },
	}
	for name, finish := range operations {
		for _, scenario := range []string{"success", "retry", "cancelled"} {
			t.Run(name+"/"+scenario, func(t *testing.T) {
				failFirst := scenario != "success"
				active := newFakeConn()
				attempts := 0
				launcher := &fakeLauncher{
					attachFn: func(_ context.Context, ticket helperclient.Ticket) (Conn, *fortivpn.Event, error) {
						active.mu.Lock()
						closed := active.closed
						active.mu.Unlock()
						if ticket.Protocol != 1 || closed {
							return nil, nil, os.ErrNotExist
						}
						return active, &fortivpn.Event{Phase: fortivpn.PhaseConnected}, nil
					},
					launchFn: func(_ context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
						attempts++
						if !req.RecoveryOnly || string(req.RecoveryJournal) != "[]" || req.Config.Host != "" {
							t.Fatal("cleanup must only recover the legacy journal")
						}
						if scenario == "cancelled" && attempts == 1 {
							return nil, helperclient.Ticket{}, helperclient.ErrElevationCancelled
						}
						ticket := helperclient.Ticket{
							Protocol: ipc.ProtocolVersion, Endpoint: "/tmp/synthetic-recovery", Token: "synthetic",
							ProfileID: req.ProfileID, ProfileName: req.ProfileName,
						}
						if err := req.OnReady(ticket); err != nil {
							return nil, ticket, err
						}
						conn := newFakeConn()
						ev := fortivpn.Event{Phase: fortivpn.PhaseDisconnected}
						if failFirst && attempts == 1 {
							ev.Err = fortivpn.NewError(fortivpn.CodeNetcfgFailed, "synthetic recovery failure")
						}
						conn.sendEvent(ev)
						return conn, ticket, nil
					},
				}
				svc, prof := setupTestService(t, launcher)
				if err := os.MkdirAll(filepath.Dir(storage.LogPath(svc.rootDir, prof.ID)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := svc.tickets.Save(helperclient.Ticket{
					Protocol: 1, Endpoint: "/tmp/synthetic-legacy", Token: "synthetic",
					ProfileID: prof.ID, ProfileName: prof.Name,
				}); err != nil {
					t.Fatal(err)
				}
				journal := storage.LegacyJournalPath(svc.rootDir)
				if err := storage.WriteFile(journal, []byte("[]"), 0o600); err != nil {
					t.Fatal(err)
				}
				if st := svc.Recover(context.Background()); st.Status != StatusConnected {
					t.Fatalf("legacy session not adopted: %+v", st)
				}
				if failFirst {
					if err := finish(svc); err == nil {
						t.Fatal("cleanup failure must block successful completion")
					}
					if _, err := os.Stat(journal); err != nil || !svc.tickets.HasPending() {
						t.Fatal("failed cleanup lost its journal or recovery ticket")
					}
					if st := svc.State(); st.Error == nil || st.ProfileID != prof.ID {
						t.Fatalf("cleanup failure must remain visible for the original profile: %+v", st)
					}
				}
				if err := finish(svc); err != nil {
					t.Fatalf("cleanup did not finish: %v", err)
				}
				wantAttempts := 1
				if failFirst {
					wantAttempts = 2
				}
				if attempts != wantAttempts || active.stopCount() != 1 {
					t.Fatalf("unexpected cleanup attempts: recovery=%d stop=%d", attempts, active.stopCount())
				}
				if _, err := os.Stat(journal); !errors.Is(err, os.ErrNotExist) || svc.tickets.HasPending() {
					t.Fatal("successful cleanup retained legacy recovery data")
				}
				if st := svc.State(); st.Status != StatusDisconnected || st.Error != nil {
					t.Fatalf("cleanup did not converge: %+v", st)
				}
				svc.EndUpdate(false)
			})
		}
	}
}

func TestLegacyRecoveryPreLaunchFailureSurfacesErrorAndBlocksStaleClear(t *testing.T) {
	svc, prof := setupTestService(t, &fakeLauncher{
		launchFn: func(context.Context, helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
			return nil, helperclient.Ticket{}, helperclient.ErrElevationCancelled
		},
	})
	legacy := storage.LegacyJournalPath(svc.rootDir)
	if err := storage.WriteFile(legacy, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := svc.Connect(context.Background(), prof.ID); err == nil {
		t.Fatal("expected Connect to abort when legacy network recovery is cancelled")
	}
	st := svc.State()
	if st.Status != StatusDisconnected || st.Error == nil {
		t.Fatalf("expected visible legacy recovery error after failed Connect preflight, got %+v", st)
	}

	svc.failDisconnect(prof.ID, prof.Name, st.Error)
	if !svc.InUse(prof.ID) {
		t.Fatal("expected profile with unresolved legacy journal to remain InUse")
	}
	svc.SyncSavedProfile(prof)
	if svc.State().Error == nil {
		t.Fatal("SyncSavedProfile must not clear error while legacy journal still exists")
	}
}
