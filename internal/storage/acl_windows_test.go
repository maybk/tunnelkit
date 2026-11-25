//go:build windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsProtectedDACLAndBroadPermissionDetection(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "secrets.json")

	if err := WritePrivateFile(target, []byte(`{"schemaVersion":1,"data":{}}`+"\n")); err != nil {
		t.Fatalf("WritePrivateFile failed: %v", err)
	}
	h, err := openRegularHandleNoFollow(target, windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		t.Fatalf("openRegularHandleNoFollow failed: %v", err)
	}
	supportsACL := volumeSupportsPersistentACLs(h)
	_ = windows.CloseHandle(h)
	if !supportsACL {
		t.Skip("temporary volume does not support persistent NTFS ACLs")
	}

	if HasBroadPermissions(target) {
		t.Fatal("expected newly created private file not to have broad Windows ACLs")
	}
	if err := RestrictPrivateFile(dir); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("RestrictPrivateFile(dir) err = %v, want os.ErrInvalid", err)
	}

	// 向目标文件注入 BUILTIN\Users 读权限，验证宽权限检测能够精确捕获。
	usersSID, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(Users): %v", err)
	}
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(usersSID)
	sd, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo: %v", err)
	}
	oldDACL, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("sd.DACL: %v", err)
	}
	broadDACL, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.FILE_GENERIC_READ,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_ALIAS,
			TrusteeValue: windows.TrusteeValueFromSID(usersSID),
		},
	}}, oldDACL)
	if err != nil {
		t.Fatalf("ACLFromEntries: %v", err)
	}
	if err := windows.SetNamedSecurityInfo(
		target,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, broadDACL, nil,
	); err != nil {
		t.Fatalf("SetNamedSecurityInfo(broad): %v", err)
	}
	if !HasBroadPermissions(target) {
		t.Fatal("expected file with BUILTIN\\Users read ACE to report broad permissions")
	}

	// 若当前环境支持创建符号链接，验证重解析点被拒绝且不篡改目标文件 ACL。
	linkPath := filepath.Join(dir, "link-secrets.json")
	if err := os.Symlink(target, linkPath); err == nil {
		if HasBroadPermissions(linkPath) {
			t.Fatal("expected symlink not to report broad Windows ACLs")
		}
		if err := RestrictPrivateFile(linkPath); !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("RestrictPrivateFile(symlink) err = %v, want os.ErrInvalid", err)
		}
		if !HasBroadPermissions(target) {
			t.Fatal("RestrictPrivateFile(symlink) must not mutate symlink target ACL")
		}
	}

	// 直接调用 RestrictPrivateFile 必须设置 SE_DACL_PROTECTED 并消除宽权限暴露。
	if err := RestrictPrivateFile(target); err != nil {
		t.Fatalf("RestrictPrivateFile failed: %v", err)
	}
	if HasBroadPermissions(target) {
		t.Fatal("expected HasBroadPermissions=false after RestrictPrivateFile")
	}
	sdAfter, err := windows.GetNamedSecurityInfo(target, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo after restrict: %v", err)
	}
	ctrl, _, err := sdAfter.Control()
	if err != nil {
		t.Fatalf("sd.Control: %v", err)
	}
	if (ctrl & windows.SE_DACL_PROTECTED) == 0 {
		t.Fatal("expected SE_DACL_PROTECTED to be set on restricted file")
	}

	// 模拟从只读介质复制后带有只读属性与宽权限的目标文件，验证再次原子写入能覆盖并重新收紧 ACL。
	if err := windows.SetNamedSecurityInfo(
		target,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, broadDACL, nil,
	); err != nil {
		t.Fatalf("SetNamedSecurityInfo(re-broad): %v", err)
	}
	if err := os.Chmod(target, 0o400); err != nil {
		t.Fatalf("Chmod readonly: %v", err)
	}
	if err := WritePrivateFile(target, []byte(`{"schemaVersion":1,"data":{"ok":"1"}}`+"\n")); err != nil {
		t.Fatalf("WritePrivateFile over readonly target failed: %v", err)
	}
	if HasBroadPermissions(target) {
		t.Fatal("expected re-saved private file ACL to be tightened back to trusted SIDs only")
	}
}
