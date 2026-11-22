//go:build darwin

package update

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDarwinBundleDiscoveryAndReplace(t *testing.T) {
	tmpDir := t.TempDir()

	mountDir := filepath.Join(tmpDir, "mount")
	srcApp := filepath.Join(mountDir, "tunnelkit.app")
	srcMacOS := filepath.Join(srcApp, "Contents", "MacOS")
	if err := os.MkdirAll(srcMacOS, 0o755); err != nil {
		t.Fatalf("mkdir srcMacOS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcMacOS, "tunnelkit"), []byte("new-mac-gui"), 0o755); err != nil {
		t.Fatalf("write src gui: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcMacOS, "tunnelkit-helper"), []byte("new-mac-helper"), 0o755); err != nil {
		t.Fatalf("write src helper: %v", err)
	}

	found, err := findAppBundleInMount(mountDir)
	if err != nil || found != srcApp {
		t.Fatalf("findAppBundleInMount: got %q, want %q (err=%v)", found, srcApp, err)
	}

	appsDir := filepath.Join(tmpDir, "Applications")
	dstApp := filepath.Join(appsDir, "tunnelkit.app")
	dstMacOS := filepath.Join(dstApp, "Contents", "MacOS")
	if err := os.MkdirAll(dstMacOS, 0o755); err != nil {
		t.Fatalf("mkdir dstMacOS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dstMacOS, "tunnelkit"), []byte("old-mac-gui"), 0o755); err != nil {
		t.Fatalf("write old gui: %v", err)
	}
	past := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(dstApp, past, past); err != nil {
		t.Fatalf("chtimes old dstApp: %v", err)
	}
	oldInfo, err := os.Stat(dstApp)
	if err != nil {
		t.Fatalf("stat old dstApp: %v", err)
	}
	oldIno := oldInfo.Sys().(*syscall.Stat_t).Ino

	// 1. 源包缺失时，原应用包内容与 inode 必须保持原样且无残留目录。
	if err := replaceMacAppBundle(t.Context(), filepath.Join(mountDir, "missing.app"), dstApp); err == nil {
		t.Fatal("expected error when source bundle does not exist")
	}
	preservedInfo, err := os.Stat(dstApp)
	if err != nil {
		t.Fatalf("expected original dstApp to remain intact: %v", err)
	}
	if gotIno := preservedInfo.Sys().(*syscall.Stat_t).Ino; gotIno != oldIno {
		t.Fatalf("expected failed staging not to touch dstApp inode: got %d, want %d", gotIno, oldIno)
	}

	// 2. 正常原子替换后，新内容生效、mtime 刷新，且旧包与隐藏暂存目录彻底清除。
	if err := replaceMacAppBundle(t.Context(), srcApp, dstApp); err != nil {
		t.Fatalf("replaceMacAppBundle failed: %v", err)
	}

	updatedGUI, err := os.ReadFile(filepath.Join(dstMacOS, "tunnelkit"))
	if err != nil || string(updatedGUI) != "new-mac-gui" {
		t.Fatalf("unexpected updated gui: %q (err=%v)", updatedGUI, err)
	}
	newInfo, err := os.Stat(dstApp)
	if err != nil {
		t.Fatalf("stat updated dstApp: %v", err)
	}
	if !newInfo.ModTime().After(past) {
		t.Fatalf("expected bundle mtime to be refreshed for LaunchServices: got %v, past %v", newInfo.ModTime(), past)
	}

	entries, err := os.ReadDir(appsDir)
	if err != nil {
		t.Fatalf("readdir appsDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "tunnelkit.app" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only tunnelkit.app in Applications dir (no .bak or .tunnelkit.app.stage-*), got %v", names)
	}

	if got := shellQuote("/Applications/it's.app"); got != "'/Applications/it'\\''s.app'" {
		t.Fatalf("unexpected shellQuote: %q", got)
	}
}

func TestSwapMacAppBundlesFallbackAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	dstApp := filepath.Join(tmpDir, "TunnelKit.app")
	bakApp := dstApp + ".bak"
	stageApp := filepath.Join(tmpDir, "staged.app")

	if err := os.MkdirAll(dstApp, 0o755); err != nil {
		t.Fatalf("mkdir dstApp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dstApp, "marker"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old marker: %v", err)
	}

	// 1. 暂存源缺失时，回退替换失败并自动将备份还原回原路径。
	if err := swapMacAppBundlesFallback(filepath.Join(tmpDir, "missing.app"), dstApp, bakApp); err == nil {
		t.Fatal("expected fallback swap to fail when staged bundle is missing")
	}
	gotOld, err := os.ReadFile(filepath.Join(dstApp, "marker"))
	if err != nil || string(gotOld) != "old" {
		t.Fatalf("expected rollback to restore old bundle, got %q (err=%v)", gotOld, err)
	}

	// 2. 暂存源就绪时，回退替换成功并清理 .bak。
	if err := os.MkdirAll(stageApp, 0o755); err != nil {
		t.Fatalf("mkdir stageApp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageApp, "marker"), []byte("new"), 0o644); err != nil {
		t.Fatalf("write new marker: %v", err)
	}
	if err := swapMacAppBundlesFallback(stageApp, dstApp, bakApp); err != nil {
		t.Fatalf("swapMacAppBundlesFallback failed: %v", err)
	}
	gotNew, err := os.ReadFile(filepath.Join(dstApp, "marker"))
	if err != nil || string(gotNew) != "new" {
		t.Fatalf("expected new bundle content, got %q (err=%v)", gotNew, err)
	}
	if _, err := os.Stat(bakApp); !os.IsNotExist(err) {
		t.Fatalf("expected backup path to be removed, err=%v", err)
	}

	// 3. CleanMacBundleArtifacts 清理异常遗留目录。
	staleStage := filepath.Join(tmpDir, ".TunnelKit.app.stage-leftover")
	_ = os.MkdirAll(staleStage, 0o755)
	_ = os.MkdirAll(dstApp+".stage", 0o755)
	_ = os.MkdirAll(bakApp, 0o755)
	CleanMacBundleArtifacts(dstApp)
	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".TunnelKit.app.stage-") || strings.HasSuffix(entry.Name(), ".bak") || strings.HasSuffix(entry.Name(), ".stage") {
			t.Fatalf("expected artifact %q to be cleaned up", entry.Name())
		}
	}
}

func TestStartMacRelaunchValidation(t *testing.T) {
	if err := startMacRelaunch(0, "/Applications/TunnelKit.app"); err == nil {
		t.Fatal("expected error for invalid PID 0")
	}
	if err := startMacRelaunch(1, "/Applications/TunnelKit.app"); err == nil {
		t.Fatal("expected error for launchd PID 1")
	}
	if err := startMacRelaunch(os.Getpid(), "   "); err == nil {
		t.Fatal("expected error for empty bundle path")
	}
}
