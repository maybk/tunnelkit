//go:build windows

package privilege

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isProcessAlive(pid int) (bool, error) {
	if pid <= 0 || int64(pid) > math.MaxUint32 {
		return false, nil
	}
	pid32 := uint32(pid) // #nosec G115 -- bounded to [1, math.MaxUint32] above.
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid32)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// The GUI may observe a LocalSystem helper without permission to open it.
		return processInSnapshot(pid32)
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, err
	}
	switch state {
	case windows.WAIT_OBJECT_0:
		return false, nil
	case uint32(windows.WAIT_TIMEOUT):
		return true, nil
	default:
		return false, fmt.Errorf("unexpected process wait state: %d", state)
	}
}

func processInSnapshot(pid uint32) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return true, nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, nil
	}
	return false, err
}
