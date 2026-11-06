package netcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	entries, err := decodeJournal(data)
	if err != nil {
		return nil, err
	}
	j.list = entries
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
	previous := j.list
	j.list = append(j.list, entry)
	if err := j.flushLocked(); err != nil {
		j.list = previous
		return err
	}
	return nil
}

// Remove deletes a specific entry (e.g. when a pre-existing host route was not
// actually created by us) and flushes the updated journal to disk.
func (j *Journal) Remove(entry JournalEntry) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	previous := j.list
	filtered := make([]JournalEntry, 0, len(j.list))
	changed := false
	for _, existing := range j.list {
		if existing == entry {
			changed = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !changed {
		return nil
	}
	j.list = filtered
	if err := j.flushLocked(); err != nil {
		j.list = previous
		return err
	}
	return nil
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
	if j.path != "" {
		if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	j.list = nil
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
	file, err := os.CreateTemp(dir, ".netcfg-journal-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp, j.path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		parent, err := os.Open(dir)
		if err != nil {
			return err
		}
		defer parent.Close()
		return parent.Sync()
	}
	return nil
}
