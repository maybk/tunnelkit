package helperclient

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tunnelkit/internal/ipc"
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
		Protocol:    ipc.ProtocolVersion,
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

func TestTicketStoreMigratesLegacyTokenBeforeReturning(t *testing.T) {
	root := t.TempDir()
	store := NewTicketStore(root)
	legacy := map[string]any{"endpoint": "/tmp/isolated-helper.sock", "token": "synthetic-legacy-session", "pid": 12345, "profileId": "legacy-profile"}
	data, _ := json.Marshal(legacy)
	if err := os.MkdirAll(filepath.Dir(store.path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != legacy["token"] || got.Protocol != 1 {
		t.Fatal("legacy session identity was not preserved")
	}
	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), legacy["token"].(string)) {
		t.Fatal("migration left a plaintext session token")
	}
	again, err := store.Load()
	if err != nil || again != got {
		t.Fatal("migrated ticket did not survive another restart")
	}
	after, _ := os.ReadFile(store.path)
	if string(after) != string(raw) {
		t.Fatal("current ticket was needlessly migrated again")
	}
	var version struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(raw, &version); err != nil || version.SchemaVersion != 1 {
		t.Fatal("ticket migration did not persist the format version", err)
	}
}

func TestTicketStoreMigratesEarlierEncryptedProtocol(t *testing.T) {
	root := t.TempDir()
	store := NewTicketStore(root)
	ticket := Ticket{Endpoint: "/tmp/synthetic-encrypted-session", Token: "synthetic-encrypted-token"}
	if err := store.Save(ticket); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &stored); err != nil {
		t.Fatal(err)
	}
	delete(stored, "protocol")
	raw, err = json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFile(store.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || got.Protocol != 2 || got.Token != ticket.Token {
		t.Fatal("earlier encrypted identity was not preserved", err)
	}
	again, err := store.Load()
	if err != nil || again != got {
		t.Fatal("protocol migration did not persist", err)
	}
}

func TestFutureTicketSchemaDoesNotChangeSavedSession(t *testing.T) {
	store := NewTicketStore(t.TempDir())
	raw := []byte(`{"schemaVersion":99,"data":{}}`)
	if err := storage.WriteFile(store.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("future ticket accepted")
	}
	if _, err := os.Stat(storage.SecretKeyPath(filepath.Dir(filepath.Dir(store.path)))); err == nil {
		t.Fatal("future version created a key")
	}
	after, _ := os.ReadFile(store.path)
	if string(after) != string(raw) {
		t.Fatal("future ticket overwritten")
	}
}

func TestFailedTicketMigrationDoesNotExposeTokenOrOverwriteSource(t *testing.T) {
	root := t.TempDir()
	store := NewTicketStore(root)
	raw := []byte(`{"endpoint":"/tmp/synthetic-session","token":"synthetic-token","pid":12345}`)
	if err := storage.WriteFile(store.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFile(storage.SecretKeyPath(root), []byte("invalid-existing-key"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err == nil || got.Token != "" {
		t.Fatal("failed migration returned a session token")
	}
	after, _ := os.ReadFile(store.path)
	if string(after) != string(raw) {
		t.Fatal("failed migration overwrote the original ticket")
	}
}

func TestTicketStoreRejectsMalformedVersionedOrOutOfBoundsPayloads(t *testing.T) {
	for _, raw := range []string{
		`{"schemaVersion":1,"data":{"protocol":1,"endpoint":"/tmp/s","token":"plaintext-secret","pid":123}}`,
		`{"schemaVersion":1,"data":{"protocol":3,"endpoint":"/tmp/s","sealedToken":{"nonce":"aa","ciphertext":"bb"},"extra":true}}`,
		`{"schemaVersion":1,"data":{"protocol":99,"endpoint":"/tmp/s","sealedToken":{"nonce":"aa","ciphertext":"bb"}}}`,
		`{"protocol":99,"endpoint":"/tmp/s","sealedToken":{"nonce":"aa","ciphertext":"bb"}}`,
		`{"protocol":2,"endpoint":"/tmp/s","sealedToken":{}}`,
	} {
		root := t.TempDir()
		store := NewTicketStore(root)
		if err := storage.WriteFile(store.path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatalf("expected invalid ticket %s to be rejected", raw)
		}
		if _, err := os.Stat(storage.SecretKeyPath(root)); err == nil {
			t.Fatalf("rejected ticket %s must not create secret.key", raw)
		}
		after, _ := os.ReadFile(store.path)
		if string(after) != raw {
			t.Fatalf("rejected ticket %s must not overwrite original file", raw)
		}
	}

	t.Run("oversized and non-regular ticket files", func(t *testing.T) {
		root := t.TempDir()
		store := NewTicketStore(root)
		oversized := `{"endpoint":"/tmp/s","token":"` + strings.Repeat("a", maxTicketFileBytes) + `"}`
		if err := storage.WriteFile(store.path, []byte(oversized), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatal("expected oversized ticket file to be rejected")
		}
		if err := os.Remove(store.path); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(store.path, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatal("expected directory at ticket path to be rejected")
		}
		if _, err := os.Stat(storage.SecretKeyPath(root)); err == nil {
			t.Fatal("rejected ticket file must not create secret.key")
		}
	})
}
