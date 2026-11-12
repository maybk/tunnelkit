//go:build darwin || linux

package privilege

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnixInspectInstalledDistinguishesDriftAndHardErrors(t *testing.T) {
	t.Parallel()

	// Unprivileged temp directories are either non-root-owned or world-writable (/tmp),
	// so inspectInstalled must refuse them with a hard error rather than offering elevation.
	tmp := t.TempDir()
	fakeTarget := filepath.Join(tmp, "tunnelkit-helper")
	if err := os.WriteFile(fakeTarget, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectInstalled(fakeTarget); err == nil {
		t.Fatal("expected hard error when helper parent chain is not root-protected")
	}

	// Missing helper under a root-owned system root must report a recoverable reason without error.
	rootOwnedBase := "/usr/libexec"
	if runtime.GOOS == "darwin" {
		rootOwnedBase = "/Library"
	}
	if err := checkProtected(rootOwnedBase, true); err != nil {
		t.Skipf("system directory %s has non-standard permissions in test environment: %v", rootOwnedBase, err)
	}
	missingTarget := filepath.Join(rootOwnedBase, "tunnelkit-nonexistent-test-dir", "tunnelkit-helper")
	reason, err := inspectInstalled(missingTarget)
	if err != nil {
		t.Fatalf("missing helper under root-owned parent should be recoverable, got err=%v", err)
	}
	if reason == "" {
		t.Fatal("expected non-empty recoverable reason for missing helper directory")
	}
}
