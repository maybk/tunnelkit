//go:build windows

package storage

import (
	"errors"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// broadAccessMask 表示向非受信任主体暴露文件内容或允许篡改/接管权限的访问位。
const broadAccessMask = windows.FILE_READ_DATA |
	windows.FILE_WRITE_DATA |
	windows.FILE_APPEND_DATA |
	windows.DELETE |
	windows.WRITE_DAC |
	windows.WRITE_OWNER |
	windows.GENERIC_READ |
	windows.GENERIC_WRITE |
	windows.GENERIC_ALL

type trustedSIDEntry struct {
	sid         *windows.SID
	trusteeType windows.TRUSTEE_TYPE
}

// RestrictPrivateFile 在支持持久 ACL 的 Windows 卷（如 NTFS）上，通过不跟随重解析点的句柄
// 将常规文件收紧为 Protected DACL，阻断父目录继承并仅授权当前用户、SYSTEM 与 Administrators 完全控制。
func RestrictPrivateFile(path string) error {
	h, err := openRegularHandleNoFollow(path, windows.FILE_READ_ATTRIBUTES|windows.WRITE_DAC)
	if err != nil {
		if isUnsupportedACLError(err) {
			return nil
		}
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()

	if !volumeSupportsPersistentACLs(h) {
		return nil
	}
	trusted, err := trustedPrivateSIDs()
	if err != nil {
		return err
	}

	entries := make([]windows.EXPLICIT_ACCESS, 0, len(trusted))
	for _, item := range trusted {
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  item.trusteeType,
				TrusteeValue: windows.TrusteeValueFromSID(item.sid),
			},
		})
	}
	dacl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}

	// 仅更新受保护 DACL（不修改 OWNER/GROUP），避免普通账户因缺少 SeRestorePrivilege 失败。
	const secInfo = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, secInfo, nil, nil, dacl, nil)
	if isUnsupportedACLError(err) {
		return nil
	}
	if err != nil {
		return mapWindowsPathError("chmod", path, err)
	}
	return nil
}

// HasBroadPermissions 检查常规私有文件在 Windows 上是否存在宽权限暴露
// （NULL DACL 或向非受信任主体授予读写、删除或权限修改权限）。
func HasBroadPermissions(path string) bool {
	h, err := openRegularHandleNoFollow(path, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()

	if !volumeSupportsPersistentACLs(h) {
		return false
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return false
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return false
	}
	// Win32 中 NULL DACL（dacl == nil 且无错误）表示对所有人完全开放。
	if dacl == nil {
		return true
	}

	trusted, err := trustedPrivateSIDs()
	if err != nil {
		return false
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil || ace == nil {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if (ace.Header.AceFlags & windows.INHERIT_ONLY_ACE) != 0 {
			continue
		}
		if (ace.Mask & broadAccessMask) == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) //nolint:gosec // G103: Win32 ACCESS_ALLOWED_ACE 尾部紧跟变长 SID 结构
		if !sid.IsValid() {
			continue
		}
		if !isTrustedSID(sid, trusted) {
			return true
		}
	}
	return false
}

// clearReadOnlyIfRegular 清除常规目标文件可能携带的只读属性（不跟随目录或重解析点），
// 以允许从外部介质复制后的文件在 Windows 上完成原子替换。
func clearReadOnlyIfRegular(path string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return
	}
	if attrs&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return
	}
	if attrs&windows.FILE_ATTRIBUTE_READONLY == 0 {
		return
	}
	newAttrs := attrs &^ windows.FILE_ATTRIBUTE_READONLY
	if newAttrs == 0 {
		newAttrs = windows.FILE_ATTRIBUTE_NORMAL
	}
	_ = windows.SetFileAttributes(p, newAttrs)
}

func openRegularHandleNoFollow(path string, access uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(
		p,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return 0, mapWindowsPathError("open", path, err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		_ = windows.CloseHandle(h)
		return 0, mapWindowsPathError("stat", path, err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		_ = windows.CloseHandle(h)
		return 0, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	return h, nil
}

func isUnsupportedACLError(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SUPPORTED) ||
		errors.Is(err, windows.ERROR_INVALID_FUNCTION) ||
		errors.Is(err, syscall.Errno(windows.ERROR_NOT_SUPPORTED)) ||
		errors.Is(err, syscall.Errno(windows.ERROR_INVALID_FUNCTION))
}

func mapWindowsPathError(op, path string, err error) error {
	if errno, ok := errors.AsType[windows.Errno](err); ok {
		err = syscall.Errno(errno)
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}

func volumeSupportsPersistentACLs(h windows.Handle) bool {
	var fsFlags uint32
	if err := windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, &fsFlags, nil, 0); err != nil {
		return false
	}
	return (fsFlags & windows.FILE_PERSISTENT_ACLS) != 0
}

var (
	trustedSIDsMu    sync.RWMutex
	trustedSIDsCache []trustedSIDEntry
)

func trustedPrivateSIDs() ([]trustedSIDEntry, error) {
	trustedSIDsMu.RLock()
	if len(trustedSIDsCache) > 0 {
		cached := trustedSIDsCache
		trustedSIDsMu.RUnlock()
		return cached, nil
	}
	trustedSIDsMu.RUnlock()

	computed, err := computeTrustedPrivateSIDs()
	if err != nil {
		return nil, err
	}

	trustedSIDsMu.Lock()
	if len(trustedSIDsCache) == 0 {
		trustedSIDsCache = computed
	}
	cached := trustedSIDsCache
	trustedSIDsMu.Unlock()
	return cached, nil
}

func computeTrustedPrivateSIDs() ([]trustedSIDEntry, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	userSID, err := tu.User.Sid.Copy()
	if err != nil {
		return nil, err
	}
	sysSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	admSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, err
	}

	candidates := []trustedSIDEntry{
		{sid: userSID, trusteeType: windows.TRUSTEE_IS_USER},
		{sid: sysSID, trusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP},
		{sid: admSID, trusteeType: windows.TRUSTEE_IS_ALIAS},
	}
	unique := make([]trustedSIDEntry, 0, len(candidates))
	for _, c := range candidates {
		if !isTrustedSID(c.sid, unique) {
			unique = append(unique, c)
		}
	}
	return unique, nil
}

func isTrustedSID(target *windows.SID, trusted []trustedSIDEntry) bool {
	for _, item := range trusted {
		if target.Equals(item.sid) {
			return true
		}
	}
	return false
}
