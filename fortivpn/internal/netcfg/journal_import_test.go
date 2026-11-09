package netcfg

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestImportedJournalMergesDurablyInChronologicalOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	old := JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", NextHop: "192.168.1.1", Iface: "eth0"}
	if runtime.GOOS == "windows" {
		old.Iface = ""
		old.LUID = 42
	}
	recent := old
	recent.Gateway = "198.51.100.11"
	j := NewJournal(path)
	if err := j.Record(recent); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]JournalEntry{old})
	for range 2 {
		if err := ImportJournal(path, raw); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := LoadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Entries(), []JournalEntry{recent, old}) {
		t.Fatal("recovery order or idempotency changed")
	}
}

func TestImportedJournalKeepsNewerOverlappingEntriesInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.json")
	old := JournalEntry{Kind: KindHostRoute, Gateway: "198.51.100.10", Iface: "eth0", NextHop: "192.168.1.1"}
	if runtime.GOOS == "windows" {
		old.Iface = ""
		old.LUID = 42
	}
	overlap, recent := old, old
	overlap.Gateway, recent.Gateway = "198.51.100.11", "198.51.100.12"
	j := NewJournal(path)
	for _, entry := range []JournalEntry{overlap, recent} {
		if err := j.Record(entry); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal([]JournalEntry{overlap, old})
	if err != nil {
		t.Fatal(err)
	}
	if err := ImportJournal(path, raw); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Entries(), []JournalEntry{recent, overlap, old}) {
		t.Fatal("newer rollback order was lost when snapshots overlapped")
	}
}
