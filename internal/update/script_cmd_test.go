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
		`copy /y "C:\Temp\stage%%1!\tunnelkit.exe" "C:\Program Files\TunnelKit\tunnelkit.exe" >nul || goto rollback`,
		`copy /y "C:\Temp\stage%%1!\wintun.dll" "C:\Program Files\TunnelKit\wintun.dll" >nul || goto rollback`,
		`if exist "C:\Program Files\TunnelKit\wintun.dll.old"`,
		":rollback",
		"exit /b 0",
		"exit /b 1",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected Windows update script to contain %q, got:\n%s", want, script)
		}
	}

	elevated := buildWindowsFileUpdateWithHelperSync(
		`C:\Temp\stage`,
		`C:\Program Files\TunnelKit`,
		[]string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"},
		`C:\Temp\done.ok`,
		`C:\Temp\done.err`,
		`C:\Program Files\TunnelKit\tunnelkit-helper.exe`,
	)
	syncLine := `"C:\Program Files\TunnelKit\tunnelkit-helper.exe" install-service >nul 2>&1`
	if !strings.Contains(elevated, syncLine) {
		t.Fatalf("expected elevated Windows update script to sync helper service, got:\n%s", elevated)
	}
	syncIdx := strings.Index(elevated, syncLine)
	okIdx := strings.Index(elevated, "echo ok > ")
	rollbackIdx := strings.Index(elevated, ":rollback")
	if syncIdx <= 0 || okIdx <= syncIdx || rollbackIdx <= okIdx {
		t.Fatalf("unexpected Windows script section order: sync=%d ok=%d rollback=%d", syncIdx, okIdx, rollbackIdx)
	}
}
