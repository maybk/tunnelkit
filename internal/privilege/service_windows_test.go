//go:build windows

package privilege

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsInstallationDescriptors(t *testing.T) {
	for _, sddl := range []string{serviceSDDL, binarySDDL, directorySDDL, stateSDDL} {
		sd, err := windows.SecurityDescriptorFromString(sddl)
		if err != nil {
			t.Fatal(err)
		}
		if aclString(sd) == "" {
			t.Fatalf("missing DACL: %s", sddl)
		}
	}
}

func TestDescriptorBookkeepingPreservesCanonicalPermissions(t *testing.T) {
	original, err := windows.SecurityDescriptorFromString(binarySDDL)
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := windows.SecurityDescriptorFromString("D:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	if aclString(original) != aclString(inherited) {
		t.Fatal("auto-inheritance bookkeeping changed explicit permissions")
	}
}
