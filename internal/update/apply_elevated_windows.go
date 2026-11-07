//go:build windows

package update

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SHELLEXECUTEINFOW 的布局在 amd64/arm64 相同；保留进程句柄直到安装子进程退出。
type shellExecuteInfo struct {
	size, mask                        uint32
	window                            windows.Handle
	verb, file, parameters, directory *uint16
	show                              int32
	instance                          windows.Handle
	idList                            uintptr
	class                             *uint16
	classKey                          windows.Handle
	hotKey                            uint32
	icon, process                     windows.Handle
}

func runElevatedInstaller(scriptPath, directory string) error {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString("cmd.exe")
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(`/d /c ""` + scriptPath + `""`)
	if err != nil {
		return err
	}
	dir, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	info := shellExecuteInfo{
		mask:       0x40 | 0x100, // SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC
		verb:       verb,
		file:       file,
		parameters: parameters,
		directory:  dir,
		show:       windows.SW_HIDE,
	}
	info.size = uint32(unsafe.Sizeof(info))
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	ok, _, callErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("Windows UAC 授权更新失败: %w", callErr)
	}
	if info.process == 0 {
		return errors.New("无法获取提权安装进程句柄")
	}
	defer windows.CloseHandle(info.process)
	// 文件事务开始后不能取消等待并删除 staging，否则子进程可能仍在写入组件。
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("管理员提权替换更新文件失败 (exit=%d)", code)
	}
	return nil
}
