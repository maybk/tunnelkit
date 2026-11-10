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

// RunElevatedWindows launches file with parameters (and optional working directory)
// via ShellExecuteExW("runas") and waits for process exit or ctx cancellation.
// Returns ErrCancelled if the user cancels the UAC prompt.
func RunElevatedWindows(ctx context.Context, file, parameters, directory string) (uint32, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	argsPtr, err := windows.UTF16PtrFromString(parameters)
	if err != nil {
		return 0, err
	}
	var dirPtr *uint16
	if directory != "" {
		dirPtr, err = windows.UTF16PtrFromString(directory)
		if err != nil {
			return 0, err
		}
	}
	info := shellExecuteInfo{
		Mask:       0x40 | 0x100, // SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC
		Verb:       verbPtr,
		File:       filePtr,
		Parameters: argsPtr,
		Directory:  dirPtr,
		Show:       windows.SW_HIDE,
	}
	info.Size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	result, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, ErrCancelled
		}
		return 0, fmt.Errorf("管理员授权操作失败: %w", callErr)
	}
	if info.Process == 0 {
		return 0, errors.New("未获得提权子进程句柄")
	}
	defer windows.CloseHandle(info.Process)
	for {
		status, err := windows.WaitForSingleObject(info.Process, 50)
		if err != nil {
			return 0, err
		}
		if status == windows.WAIT_OBJECT_0 {
			var code uint32
			if err := windows.GetExitCodeProcess(info.Process, &code); err != nil {
				return 0, err
			}
			return code, nil
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
}

func runElevatedAndWait(ctx context.Context, source, command string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	code, err := RunElevatedWindows(waitCtx, source, command, "")
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("辅助组件管理失败（退出码 %d）", code)
	}
	return nil
}

func elevateInstall(ctx context.Context, source string) error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return InstallService(source)
	}
	return runElevatedAndWait(ctx, source, "install-service")
}
