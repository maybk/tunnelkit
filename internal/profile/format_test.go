package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"tunnelkit/internal/storage"
)

func TestLegacyDocumentsRemainReadableAndBecomeVersionedOnSave(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	p, err := store.Save(Input{Name: "migration", Host: "vpn.example.com", Username: "u", Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	// Model the two unversioned files produced by the released application.
	for _, path := range []string{storage.ProfilesPath(root), storage.SecretsPath(root)} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(raw, &envelope)
		if len(envelope.Data) > 0 {
			raw = envelope.Data
		}
		if err := storage.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, pwd, err := store.GetWithPassword(p.ID); err != nil || pwd != "synthetic-password" {
		t.Fatal("legacy credentials lost", err)
	}
	if _, err := store.Save(Input{ID: p.ID, Name: "updated", Host: p.Host, Username: p.Username, Password: "synthetic-replacement"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{storage.ProfilesPath(root), storage.SecretsPath(root)} {
		raw, _ := os.ReadFile(path)
		var h struct {
			Version int `json:"schemaVersion"`
		}
		_ = json.Unmarshal(raw, &h)
		if h.Version != 1 {
			t.Fatal("missing file schema version", path)
		}
	}
}

func TestFutureProfileAndCredentialFormatsArePreserved(t *testing.T) {
	for _, file := range []string{"profiles", "secrets"} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)
			p, err := store.Save(Input{Name: "migration", Host: "vpn.example.com", Username: "u", Password: "synthetic-password"})
			if err != nil {
				t.Fatal(err)
			}
			path := storage.ProfilesPath(root)
			if file == "secrets" {
				path = storage.SecretsPath(root)
			}
			raw := []byte(`{"schemaVersion":99,"data":{}}`)
			if err := storage.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Save(Input{ID: p.ID, Name: p.Name, Host: p.Host, Username: p.Username, Password: "synthetic-replacement"}); err == nil {
				t.Fatal("future format accepted")
			}
			if _, err := store.List(); err == nil {
				t.Fatal("future format was hidden from the caller")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("future document overwritten")
			}
		})
	}
}
