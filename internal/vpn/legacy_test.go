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
