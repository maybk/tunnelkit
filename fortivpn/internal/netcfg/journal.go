package netcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// JournalEntryKind identifies a persistent host network resource recorded for crash recovery.
type JournalEntryKind string

const (
	KindHostRoute    JournalEntryKind = "host_route"
	KindDarwinDNSKey JournalEntryKind = "darwin_dns_key"
	KindWindowsNRPT  JournalEntryKind = "windows_nrpt"
	KindLinuxDNS     JournalEntryKind = "linux_dns"
)

// JournalEntry describes a single host change that does not automatically vanish
// when the helper process or TUN interface exits.
type JournalEntry struct {
	Kind    JournalEntryKind `json:"kind"`
	Gateway string           `json:"gateway,omitempty"`
	NextHop string           `json:"nextHop,omitempty"`
	Iface   string           `json:"iface,omitempty"`
	LUID    uint64           `json:"luid,omitempty"`
	Key     string           `json:"key,omitempty"`
}

// Journal manages write-ahead persistence of rollback entries.
// When path is empty, entries are kept in memory only.
type Journal struct {
	path string
	mu   sync.Mutex
	list []JournalEntry
}

// NewJournal creates a Journal backed by path (or in-memory if path == "").
func NewJournal(path string) *Journal {
	return &Journal{path: path}
}

// LoadJournal reads an existing journal file from disk. If the file does not
// exist, an empty Journal is returned without error.
func LoadJournal(path string) (*Journal, error) {
	j := &Journal{path: path}
	if path == "" {
		return j, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return j, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return j, nil
	}
	if err := json.Unmarshal(data, &j.list); err != nil {
		return nil, err
	}
	return j, nil
}

// Record appends an entry (ignoring exact duplicates) and flushes the journal
// to disk atomically BEFORE the caller mutates the system.
func (j *Journal) Record(entry JournalEntry) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, existing := range j.list {
		if existing == entry {
			return nil
		}
	}
	j.list = append(j.list, entry)
	return j.flushLocked()
}

// Remove deletes a specific entry (e.g. when a pre-existing host route was not
// actually created by us) and flushes the updated journal to disk.
func (j *Journal) Remove(entry JournalEntry) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	filtered := j.list[:0]
	changed := false
	for _, existing := range j.list {
		if existing == entry {
			changed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	j.list = filtered
	if !changed {
		return nil
	}
	return j.flushLocked()
}

// Entries returns a copy of the recorded entries in reverse order (LIFO rollback order).
func (j *Journal) Entries() []JournalEntry {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]JournalEntry, len(j.list))
	for i := range j.list {
		out[i] = j.list[len(j.list)-1-i]
	}
	return out
}

// Clear removes all recorded entries and deletes the journal file.
func (j *Journal) Clear() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.list = nil
	if j.path == "" {
		return nil
	}
	if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (j *Journal) flushLocked() error {
	if j.path == "" {
		return nil
	}
	dir := filepath.Dir(j.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j.list, "", "  ")
	if err != nil {
		return err
	}
	tmp := j.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, j.path)
}
