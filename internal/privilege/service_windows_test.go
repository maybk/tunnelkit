//go:build windows

package privilege

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

func TestWindowsInstallationDescriptors(t *testing.T) {
	if strings.Contains(serviceSDDL, "GA") || strings.Contains(serviceSDDL, "GR") || strings.Contains(serviceSDDL, "GW") || strings.Contains(serviceSDDL, "GX") {
		t.Fatalf("serviceSDDL must use explicit service access rights instead of generic masks so SCM round-trips identically: %s", serviceSDDL)
	}
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

func TestWindowsFileAndDirectoryACLSelfHealing(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires elevated Windows token")
	}

	root := t.TempDir()

	// 1. Fresh protected directory and binary round-trip.
	freshDir := filepath.Join(root, "fresh-dir")
	if err := ensureProtectedDirectory(freshDir, directorySDDL); err != nil {
		t.Fatalf("ensureProtectedDirectory fresh: %v", err)
	}
	binFile := filepath.Join(freshDir, "tunnelkit-helper.exe")
	if err := os.WriteFile(binFile, []byte("helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := setACL(binFile, binarySDDL); err != nil {
		t.Fatalf("setACL binary: %v", err)
	}
	if err := verifyACL(binFile, binarySDDL); err != nil {
		t.Fatalf("verifyACL binary: %v", err)
	}

	// 2. Existing directory with Administrators owner but outdated/inherited DACL self-heals.
	driftedDir := filepath.Join(root, "drifted-state")
	if err := os.MkdirAll(driftedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Assign Administrators owner with a permissive/non-matching DACL first, simulating
	// an older release or LocalSystem MkdirAll directory with inherited ACEs.
	if err := setACL(driftedDir, directorySDDL); err != nil {
		t.Fatal(err)
	}
	if err := verifyACL(driftedDir, stateSDDL); err == nil {
		t.Fatal("expected verifyACL with stateSDDL to fail before repair")
	}
	if err := ensureProtectedDirectory(driftedDir, stateSDDL); err != nil {
		t.Fatalf("ensureProtectedDirectory should repair existing trusted directory ACL: %v", err)
	}
	if err := verifyACL(driftedDir, stateSDDL); err != nil {
		t.Fatalf("verifyACL after repair: %v", err)
	}

	// 3. Reparse point (junction) must be rejected as a hard error.
	targetDir := filepath.Join(root, "real-target")
	junctionDir := filepath.Join(root, "junction-link")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junctionDir, targetDir).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J failed: %v (%s)", err, out)
	}
	if err := ensureProtectedDirectory(junctionDir, directorySDDL); err == nil || !strings.Contains(err.Error(), "reparse points") {
		t.Fatalf("expected junction to be rejected as reparse point, got %v", err)
	}
}

func TestWindowsServiceSDDLRoundTripWithSCM(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires elevated Windows token to register temporary SCM test service")
	}

	selfExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m, err := mgr.Connect()
	if err != nil {
		t.Fatalf("connect SCM: %v", err)
	}
	defer m.Disconnect()

	svcName := fmt.Sprintf("TunnelKitHelperTest_%d", os.Getpid())
	if existing, openErr := m.OpenService(svcName); openErr == nil {
		_ = existing.Delete()
		_ = existing.Close()
	}

	cfg := mgr.Config{
		ServiceType:      windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:        mgr.StartManual,
		ErrorControl:     mgr.ErrorNormal,
		BinaryPathName:   windows.EscapeArg(selfExe),
		ServiceStartName: "LocalSystem",
		DisplayName:      "TunnelKit Helper SDDL RoundTrip Test",
	}
	s, err := m.CreateService(svcName, selfExe, cfg)
	if err != nil {
		t.Fatalf("CreateService(%s): %v", svcName, err)
	}
	t.Cleanup(func() {
		_ = s.Delete()
		_ = s.Close()
	})

	if err := grantServiceStartACL(s.Handle); err != nil {
		t.Fatalf("grantServiceStartACL: %v", err)
	}

	reason, err := checkServiceHandle(s, selfExe)
	if err != nil {
		t.Fatalf("checkServiceHandle returned unexpected error: %v", err)
	}
	if reason != "" {
		t.Fatalf("checkServiceHandle reported mismatch after grantServiceStartACL: %s", reason)
	}

	// Drift StartType -> checkServiceHandle must report recoverable reason without hard error.
	cfg.StartType = mgr.StartDisabled
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig(StartDisabled): %v", err)
	}
	reason, err = checkServiceHandle(s, selfExe)
	if err != nil || reason == "" {
		t.Fatalf("expected recoverable StartType mismatch reason, got reason=%q err=%v", reason, err)
	}
}

func TestSmokeInstalledWindowsHelperIsCurrent(t *testing.T) {
	src := os.Getenv("TUNNELKIT_SMOKE_HELPER_SOURCE")
	if src == "" {
		t.Skip("TUNNELKIT_SMOKE_HELPER_SOURCE not set")
	}
	ok, err := Current(src)
	if err != nil {
		t.Fatalf("Current(%q) returned error: %v", src, err)
	}
	if !ok {
		absSrc, _ := filepath.Abs(src)
		reason, inspectErr := inspectCurrent(absSrc)
		t.Fatalf("Current(%q) = false after installer ran (reason=%q, err=%v)", src, reason, inspectErr)
	}
}
