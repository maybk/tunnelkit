package update

import (
	"strings"
	"testing"
)

func TestBuildWindowsFileUpdateStructure(t *testing.T) {
	script := buildWindowsFileUpdate(
		"C:\\Temp\\stage%1!\r\n\"",
		`C:\Program Files\TunnelKit`,
		[]string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"},
		`C:\Temp\done.ok`,
		`C:\Temp\done.err`,
	)
	for _, want := range []string{
		"setlocal DisableDelayedExpansion",
		`copy /y "C:\Temp\stage%%1!\tunnelkit.exe" "C:\Program Files\TunnelKit\tunnelkit.exe.new" >nul || goto rollback`,
		`copy /y "C:\Temp\stage%%1!\wintun.dll" "C:\Program Files\TunnelKit\wintun.dll.new" >nul || goto rollback`,
		`move /y "C:\Program Files\TunnelKit\tunnelkit.exe.new" "C:\Program Files\TunnelKit\tunnelkit.exe" >nul || goto rollback`,
		`if exist "C:\Program Files\TunnelKit\wintun.dll.old"`,
		":rollback",
		"exit /b 0",
		"exit /b 1",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected Windows update script to contain %q, got:\n%s", want, script)
		}
	}

	sum := strings.Repeat("a", 64)
	elevated := buildWindowsFileUpdateWithHelperSync(
		`C:\Temp\stage`,
		`C:\Program Files\TunnelKit`,
		[]string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"},
		`C:\Temp\done.ok`,
		`C:\Temp\done.err`,
		`C:\Program Files\TunnelKit\tunnelkit-helper.exe`,
		map[string]string{
			"tunnelkit.exe": sum,
		},
	)
	verifyLine := `"%SystemRoot%\System32\certutil.exe" -hashfile "C:\Program Files\TunnelKit\tunnelkit.exe.new" SHA256 | "%SystemRoot%\System32\findstr.exe" /i /l /x "` + sum + `" >nul || goto rollback`
	backupLine := `move /y "C:\Program Files\TunnelKit\tunnelkit.exe" "C:\Program Files\TunnelKit\tunnelkit.exe.old" >nul || goto rollback`
	syncLine := `"C:\Program Files\TunnelKit\tunnelkit-helper.exe" install-service >nul 2>&1`
	for _, want := range []string{verifyLine, backupLine, syncLine} {
		if !strings.Contains(elevated, want) {
			t.Fatalf("expected elevated Windows update script to contain %q, got:\n%s", want, elevated)
		}
	}
	verifyIdx := strings.Index(elevated, verifyLine)
	backupIdx := strings.Index(elevated, backupLine)
	syncIdx := strings.Index(elevated, syncLine)
	okIdx := strings.Index(elevated, "echo ok > ")
	rollbackIdx := strings.Index(elevated, ":rollback")
	if verifyIdx <= 0 || backupIdx <= verifyIdx || syncIdx <= backupIdx || okIdx <= syncIdx || rollbackIdx <= okIdx {
		t.Fatalf("unexpected Windows script section order: verify=%d backup=%d sync=%d ok=%d rollback=%d", verifyIdx, backupIdx, syncIdx, okIdx, rollbackIdx)
	}
}
