package netcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const journalSchemaVersion = 1

type journalDocument struct {
	SchemaVersion int            `json:"schemaVersion"`
	Entries       []JournalEntry `json:"entries"`
}

// decodeJournal accepts the released array format and the current versioned document.
// This codec stays in the independent kernel; it never imports desktop storage.
func decodeJournal(data []byte) ([]JournalEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if len(data) > 64<<10 {
		return nil, errors.New("network recovery journal is too large")
	}
	var entries []JournalEntry
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if trimmed[0] == '{' {
		var doc journalDocument
		if err := decoder.Decode(&doc); err != nil {
			return nil, err
		}
		if doc.SchemaVersion != journalSchemaVersion {
			return nil, fmt.Errorf("unsupported network journal schema version %d", doc.SchemaVersion)
		}
		if doc.Entries == nil {
			return nil, errors.New("network journal is missing entries")
		}
		entries = doc.Entries
	} else {
		if err := decoder.Decode(&entries); err != nil {
			return nil, err
		}
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("unexpected data after network recovery journal")
	}
	if err := validateJournalEntries(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func encodeJournal(entries []JournalEntry) ([]byte, error) {
	if err := validateJournalEntries(entries); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []JournalEntry{}
	}
	return json.MarshalIndent(journalDocument{journalSchemaVersion, entries}, "", "  ")
}
