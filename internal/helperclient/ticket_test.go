package helperclient

import (
	"errors"
	"os"
	"strings"
	"testing"

	"tunnelkit/internal/storage"
)

func TestTicketStoreSaveEncryptsTokenAndLoadDecrypts(t *testing.T) {
	root := t.TempDir()
	store := NewTicketStore(root)

	if err := store.Save(Ticket{Endpoint: "/tmp/tunnelkit.sock"}); err == nil {
		t.Fatal("expected Save with empty token to fail")
	}
	if err := store.Save(Ticket{Token: "plain-secret-ipc-token-12345"}); err == nil {
		t.Fatal("expected Save with empty endpoint to fail")
	}

	want := Ticket{
		Endpoint:    "/tmp/tunnelkit.sock",
		Token:       "plain-secret-ipc-token-12345",
		PID:         4242,
		ProfileID:   "prof-1",
		ProfileName: "Office VPN",
		StartedAt:   "2026-04-20T12:00:00Z",
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	raw, err := os.ReadFile(storage.TicketPath(root))
	if err != nil {
		t.Fatalf("ReadFile session.json failed: %v", err)
	}
	if strings.Contains(string(raw), want.Token) {
		t.Fatalf("session.json must not contain plaintext token: %s", string(raw))
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist after Clear, got %v", err)
	}
}
