//go:build darwin

package update

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

	dstApp := filepath.Join(tmpDir, "Applications", "tunnelkit.app")
	dstMacOS := filepath.Join(dstApp, "Contents", "MacOS")
	if err := os.MkdirAll(dstMacOS, 0o755); err != nil {
		t.Fatalf("mkdir dstMacOS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dstMacOS, "tunnelkit"), []byte("old-mac-gui"), 0o755); err != nil {
		t.Fatalf("write old gui: %v", err)
	}

	if err := replaceMacAppBundle(context.Background(), srcApp, dstApp); err != nil {
		t.Fatalf("replaceMacAppBundle failed: %v", err)
	}

	updatedGUI, err := os.ReadFile(filepath.Join(dstMacOS, "tunnelkit"))
	if err != nil || string(updatedGUI) != "new-mac-gui" {
		t.Fatalf("unexpected updated gui: %q (err=%v)", updatedGUI, err)
	}
	if _, err := os.Stat(dstApp + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected backup bundle %s.bak to be cleaned up, err=%v", dstApp, err)
	}

	if got := darwinShellQuote("/Applications/it's.app"); got != "'/Applications/it'\\''s.app'" {
		t.Fatalf("unexpected darwinShellQuote: %q", got)
	}
}
