package netcfg

import (
	"errors"
	"regexp"
	"runtime"
)

var ownedLinuxTunnel = regexp.MustCompile(`^(forti|tun)[0-9]+$`)

// ImportJournal validates the complete snapshot before merging it into protected WAL storage.
func ImportJournal(path string, data []byte) error {
	if path == "" {
		return errors.New("recovery requires a persistent journal")
	}
	entries, err := decodeJournal(data)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		valid := false
		switch entry.Kind {
		case KindHostRoute:
			valid = runtime.GOOS == "windows" && entry.LUID != 0 || (runtime.GOOS == "darwin" || runtime.GOOS == "linux") && entry.LUID == 0
		case KindDarwinDNSKey:
			valid = runtime.GOOS == "darwin"
		case KindWindowsNRPT:
			valid = runtime.GOOS == "windows"
		case KindLinuxDNS:
			valid = runtime.GOOS == "linux" && ownedLinuxTunnel.MatchString(entry.Iface)
		}
		if !valid {
			return errors.New("recovery record does not belong to a supported local VPN resource")
		}
	}
	j, err := LoadJournal(path)
	if err != nil {
		return err
	}
	merged := make([]JournalEntry, 0, len(entries)+len(j.list))
	seen := make(map[JournalEntry]bool, len(entries)+len(j.list))
	// Existing entries are newer: retain their position when snapshots overlap.
	for _, entry := range j.list {
		seen[entry] = true
	}
	for _, entry := range entries {
		if !seen[entry] {
			merged = append(merged, entry)
			seen[entry] = true
		}
	}
	merged = append(merged, j.list...)
	if len(merged) > 16 {
		return errors.New("too many combined network recovery entries")
	}
	j.list = merged
	return j.flushLocked()
}
