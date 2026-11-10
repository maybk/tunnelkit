package netcfg

import "net/netip"

// recordAndApplyHostRoute records entry in journal before invoking addFn, and removes
// the journal entry if addFn fails or reports that the route already existed.
func recordAndApplyHostRoute(journal *Journal, entry JournalEntry, addFn func() (bool, error)) error {
	if err := journal.Record(entry); err != nil {
		return err
	}
	created, err := addFn()
	if err != nil {
		_ = journal.Remove(entry)
		return err
	}
	if !created {
		_ = journal.Remove(entry)
	}
	return nil
}

func buildHostRouteEntry(gateway netip.Addr, nextHop netip.Addr, iface string, luid uint64) JournalEntry {
	entry := JournalEntry{
		Kind:    KindHostRoute,
		Gateway: gateway.String(),
		Iface:   iface,
		LUID:    luid,
	}
	if nextHop.IsValid() {
		entry.NextHop = nextHop.String()
	}
	return entry
}
