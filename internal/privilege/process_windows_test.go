//go:build windows

package privilege

import (
	"os"
	"testing"
)

func TestProcessSnapshotObservesCurrentProcess(t *testing.T) {
	alive, err := processInSnapshot(uint32(os.Getpid()))
	if err != nil || !alive {
		t.Fatalf("snapshot did not observe current process: alive=%v err=%v", alive, err)
	}
}
