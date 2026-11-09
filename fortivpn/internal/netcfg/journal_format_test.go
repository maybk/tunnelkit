package netcfg

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyJournalBecomesVersionedWithoutLosingOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	raw := []byte(`[{"kind":"host_route","gateway":"198.51.100.10","iface":"en0"}]`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	j, err := LoadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	next := JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.11", Iface: "en0"}
	if err := j.Record(next); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var h struct {
		Version int `json:"schemaVersion"`
	}
	_ = json.Unmarshal(data, &h)
	if h.Version != 1 {
		t.Fatal("missing journal schema version")
	}
	j, err = LoadJournal(path)
	if err != nil || len(j.Entries()) != 2 || j.Entries()[0] != next {
		t.Fatal("rollback order changed", err)
	}
}
func TestFutureJournalRemainsUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	raw := []byte(`{"schemaVersion":99,"entries":[]}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadJournal(path); err == nil {
		t.Fatal("future journal accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("future journal changed")
	}
}
