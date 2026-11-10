package update

import (
	"strings"
	"testing"
)

func TestBuildWindowsFileUpdateStructure(t *testing.T) {
	script := buildWindowsFileUpdate(
		`C:\Temp\stage%1`,
		`C:\Program Files\TunnelKit`,
		[]string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"},
		`C:\Temp\done.ok`,
		`C:\Temp\done.err`,
	)
	for _, want := range []string{
		"setlocal DisableDelayedExpansion",
		"%%1",
		":rollback",
		"wintun.dll.old",
		"exit /b 0",
		"exit /b 1",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected Windows update script to contain %q, got:\n%s", want, script)
		}
	}
}
