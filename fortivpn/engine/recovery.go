package engine

import "tunnelkit/fortivpn/internal/netcfg"

// ImportRecoveryJournal validates and durably merges an earlier recovery snapshot.
// It does not execute commands; Recover consumes the resulting journal afterwards.
func ImportRecoveryJournal(path string, data []byte) error { return netcfg.ImportJournal(path, data) }
