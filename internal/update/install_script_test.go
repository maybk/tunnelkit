package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildPOSIXFileUpdateSuccessAndRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test")
	}
	installBin, err := exec.LookPath("install")
	if err != nil {
		t.Skip("install binary not found")
	}

	tmp := t.TempDir()
	stageDir := filepath.Join(tmp, "stage")
	targetDir := filepath.Join(tmp, "target")
	_ = os.MkdirAll(stageDir, 0o755)
	_ = os.MkdirAll(targetDir, 0o755)

	_ = os.WriteFile(filepath.Join(targetDir, "tunnelkit"), []byte("old-gui"), 0o755)
	_ = os.WriteFile(filepath.Join(targetDir, "tunnelkit-helper"), []byte("old-helper"), 0o755)

	// 1. 缺少第二个暂存文件时，应回滚第一个已备份的文件。
	_ = os.WriteFile(filepath.Join(stageDir, "tunnelkit"), []byte("new-gui"), 0o755)
	script := buildPOSIXFileUpdate(stageDir, targetDir, []string{"tunnelkit", "tunnelkit-helper"}, installBin)
	if err := exec.Command("/bin/sh", "-c", script).Run(); err == nil {
		t.Fatal("expected script to fail when tunnelkit-helper is missing in stageDir")
	}
	gotGUI, err := os.ReadFile(filepath.Join(targetDir, "tunnelkit"))
	if err != nil || string(gotGUI) != "old-gui" {
		t.Fatalf("expected rollback to restore old-gui, got %q (err=%v)", gotGUI, err)
	}

	// 2. 两个暂存文件齐全时，应全部成功更新。
	_ = os.WriteFile(filepath.Join(stageDir, "tunnelkit-helper"), []byte("new-helper"), 0o755)
	if out, err := exec.Command("/bin/sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("expected script to succeed, got %v: %s", err, out)
	}
	gotGUI, _ = os.ReadFile(filepath.Join(targetDir, "tunnelkit"))
	gotHelper, _ := os.ReadFile(filepath.Join(targetDir, "tunnelkit-helper"))
	if string(gotGUI) != "new-gui" || string(gotHelper) != "new-helper" {
		t.Fatalf("unexpected updated contents: gui=%q helper=%q", gotGUI, gotHelper)
	}
}

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
