package privilege

import (
	"os"
	"path/filepath"
	"time"
)

// AcquireSession serializes system-wide network changes across users/processes.
// Only system administrators can create or modify this lock file.
func AcquireSession() (*os.File, error) {
	base, err := stateRoot()
	if err != nil {
		return nil, err
	}
	if err := protectDirectory(base); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(base, "session.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := lockFile(f)
		if err == nil {
			return f, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, err
		}
		time.Sleep(40 * time.Millisecond)
	}
}
