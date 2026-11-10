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

func TestBuildMacElevatedUpdateShellCmdSuccessAndRollback(t *testing.T) {
	script := buildMacElevatedUpdateShellCmd(
		"/Volumes/TunnelKit/TunnelKit's.app",
		"/Applications/TunnelKit.app",
		"/Applications/TunnelKit.app.bak",
	)
	for _, want := range []string{
		"set -eu",
		`'/Volumes/TunnelKit/TunnelKit'\''s.app'`,
		`'/Applications/TunnelKit.app'`,
		`'/Applications/TunnelKit.app.bak'`,
		"/usr/bin/ditto",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected macOS update script to contain %q, got:\n%s", want, script)
		}
	}

	if runtime.GOOS != "darwin" {
		return
	}

	tmp := t.TempDir()
	srcApp := filepath.Join(tmp, "src.app")
	dstApp := filepath.Join(tmp, "TunnelKit.app")
	bakApp := dstApp + ".bak"

	_ = os.MkdirAll(dstApp, 0o755)
	_ = os.WriteFile(filepath.Join(dstApp, "version.txt"), []byte("old-bundle"), 0o644)

	// 1. 源 .app 不存在时，ditto 失败应自动恢复原 .app。
	cmdText := buildMacElevatedUpdateShellCmd(srcApp, dstApp, bakApp)
	if err := exec.Command("/bin/sh", "-c", cmdText).Run(); err == nil {
		t.Fatal("expected script to fail when srcApp does not exist")
	}
	gotOld, err := os.ReadFile(filepath.Join(dstApp, "version.txt"))
	if err != nil || string(gotOld) != "old-bundle" {
		t.Fatalf("expected rollback to restore old-bundle, got %q (err=%v)", gotOld, err)
	}

	// 2. 源 .app 存在时，应完成替换并清理 .bak。
	_ = os.MkdirAll(srcApp, 0o755)
	_ = os.WriteFile(filepath.Join(srcApp, "version.txt"), []byte("new-bundle"), 0o644)
	if out, err := exec.Command("/bin/sh", "-c", cmdText).CombinedOutput(); err != nil {
		t.Fatalf("expected macOS update script to succeed, got %v: %s", err, out)
	}
	gotNew, err := os.ReadFile(filepath.Join(dstApp, "version.txt"))
	if err != nil || string(gotNew) != "new-bundle" {
		t.Fatalf("expected updated bundle content 'new-bundle', got %q (err=%v)", gotNew, err)
	}
	if _, err := os.Stat(bakApp); !os.IsNotExist(err) {
		t.Fatalf("expected backup bundle to be removed after success, err=%v", err)
	}
}
