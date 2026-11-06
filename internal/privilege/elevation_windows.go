//go:build windows

package privilege

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shellExecuteInfo matches SHELLEXECUTEINFOW. SEE_MASK_NOCLOSEPROCESS lets the
// caller observe the elevated installer exit status instead of polling files.
type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

func runElevatedAndWait(ctx context.Context, source, command string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{Mask: 0x40 | 0x100, Verb: verb, File: file, Parameters: args, Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		return fmt.Errorf("管理员授权操作失败: %w", callErr)
	}
	if info.Process == 0 {
		return errors.New("未获得辅助组件管理进程句柄")
	}
	defer windows.CloseHandle(info.Process)
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		status, err := windows.WaitForSingleObject(info.Process, 50)
		if err != nil {
			return err
		}
		if status == windows.WAIT_OBJECT_0 {
			var code uint32
			if err := windows.GetExitCodeProcess(info.Process, &code); err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("辅助组件管理失败（退出码 %d）", code)
			}
			return nil
		}
		if err := waitCtx.Err(); err != nil {
			return err
		}
	}
}

func elevateInstall(ctx context.Context, source string) error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return InstallService(source)
	}
	return runElevatedAndWait(ctx, source, "install-service")
}
