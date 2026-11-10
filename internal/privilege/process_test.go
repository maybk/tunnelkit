//go:build darwin || linux || windows

package privilege

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestProcessRejectsNonPositivePID(t *testing.T) {
	for _, pid := range []int{0, -1, -os.Getpid()} {
		alive, err := IsProcessAlive(pid)
		if err != nil || alive {
			t.Fatalf("PID %d must not select a process group: alive=%v err=%v", pid, alive, err)
		}
	}
}

func TestProcessObservesCurrentProcess(t *testing.T) {
	alive, err := IsProcessAlive(os.Getpid())
	if err != nil || !alive {
		t.Fatalf("current process not observed: alive=%v err=%v", alive, err)
	}
}

func TestProcessObservesReapedChild(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessChild$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	alive, err := IsProcessAlive(cmd.Process.Pid)
	if err != nil || alive {
		t.Fatalf("reaped child still observed: alive=%v err=%v", alive, err)
	}
}

func TestProcessChild(t *testing.T) {}

func TestProcessRejectsOutOfRangePID(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("requires a PID value exceeding the platform PID width")
	}
	alive, err := IsProcessAlive(int(^uint(0) >> 1))
	if err != nil || alive {
		t.Fatalf("out-of-range PID was truncated: alive=%v err=%v", alive, err)
	}
}
