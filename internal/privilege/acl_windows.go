//go:build windows

package privilege

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	binarySDDL    = "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;BU)"
	directorySDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)"
	stateSDDL     = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
)

// Compare canonical ACEs separately from descriptor bookkeeping flags (AI/AR).
func aclString(sd *windows.SECURITY_DESCRIPTOR) string {
	text := sd.String()
	if d := strings.Index(text, "D:"); d >= 0 {
		text = text[d:]
		if a := strings.Index(text, "("); a >= 0 {
			return text[a:]
		}
	}
	return ""
}

func checkDescriptor(sd *windows.SECURITY_DESCRIPTOR, wantSDDL string) (trustedOwner bool, aclOK bool, err error) {
	owner, _, err := sd.Owner()
	if err != nil {
		return false, false, err
	}
	trustedOwner = owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) || owner.IsWellKnown(windows.WinLocalSystemSid)
	want, err := windows.SecurityDescriptorFromString(wantSDDL)
	if err != nil {
		return trustedOwner, false, err
	}
	control, _, err := sd.Control()
	if err != nil {
		return trustedOwner, false, err
	}
	aclOK = control&windows.SE_DACL_PROTECTED != 0 && aclString(sd) == aclString(want)
	return trustedOwner, aclOK, nil
}

func parseAdminDACL(sddl string) (*windows.SID, *windows.ACL, error) {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, nil, err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, nil, err
	}
	owner, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, nil, err
	}
	return owner, dacl, nil
}

func verifyNoLinkOrReparse(path string, wantDir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("helper paths must not be links")
	}
	attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path))
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("helper paths must not be reparse points")
	}
	if wantDir && !info.IsDir() {
		return fmt.Errorf("invalid helper directory: %s", path)
	}
	if !wantDir && !info.Mode().IsRegular() {
		return fmt.Errorf("invalid helper component: %s", path)
	}
	return nil
}

func inspectSecurityDescriptor(path, sddl string) (trustedOwner bool, aclOK bool, err error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, false, err
	}
	return checkDescriptor(sd, sddl)
}

func verifyPathACL(path, sddl string, wantDir bool) error {
	if err := verifyNoLinkOrReparse(path, wantDir); err != nil {
		return err
	}
	trustedOwner, aclOK, err := inspectSecurityDescriptor(path, sddl)
	if err != nil {
		return err
	}
	if !trustedOwner {
		return fmt.Errorf("unsafe helper path owner: %s", path)
	}
	if !aclOK {
		return fmt.Errorf("unsafe helper path ACL: %s", path)
	}
	return nil
}

func verifyACL(path, sddl string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return verifyPathACL(path, sddl, info.IsDir())
}

func setACL(path, sddl string) error {
	owner, dacl, err := parseAdminDACL(sddl)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner,
		nil,
		dacl,
		nil,
	)
}
