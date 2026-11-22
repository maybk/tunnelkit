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

func TestResolveTrustedSystemBinary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	untrusted := filepath.Join(tmp, "fake-pkexec")
	if err := os.WriteFile(untrusted, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := ResolveTrustedSystemBinary(untrusted); ok {
		t.Fatalf("expected untrusted temp binary to be rejected, got %q", got)
	}
	if got, ok := ResolveTrustedSystemBinary("/nonexistent/bin", "/bin/sh"); !ok || got != "/bin/sh" {
		t.Fatalf("expected /bin/sh to resolve as trusted system binary, got (%q, %v)", got, ok)
	}

	// Simulate Linux usrmerge (/bin -> usr/bin) + Debian alternatives / sudo-rs multi-hop symlinks.
	uid := uint32(os.Getuid())
	sysRoot := filepath.Join(tmp, "sysroot")
	for _, rel := range []string{
		"",
		"usr",
		"usr/bin",
		"usr/lib",
		"usr/lib/cargo",
		"usr/lib/cargo/bin",
		"etc",
		"etc/alternatives",
	} {
		dir := filepath.Join(sysRoot, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("usr/bin", filepath.Join(sysRoot, "bin")); err != nil {
		t.Fatal(err)
	}
	dashBin := filepath.Join(sysRoot, "usr/bin/dash")
	if err := os.WriteFile(dashBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dashBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("dash", filepath.Join(sysRoot, "usr/bin/sh")); err != nil {
		t.Fatal(err)
	}

	wantSh := filepath.Join(sysRoot, "bin/sh")
	if got, ok := resolveTrustedBinaryAt(sysRoot, uid, "/bin/sh"); !ok || got != wantSh {
		t.Fatalf("usrmerge /bin/sh resolution = (%q, %v), want (%q, true)", got, ok, wantSh)
	}

	sudoReal := filepath.Join(sysRoot, "usr/lib/cargo/bin/sudo")
	if err := os.WriteFile(sudoReal, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sudoReal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../usr/lib/cargo/bin/sudo", filepath.Join(sysRoot, "etc/alternatives/sudo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/alternatives/sudo", filepath.Join(sysRoot, "usr/bin/sudo")); err != nil {
		t.Fatal(err)
	}
	wantSudo := filepath.Join(sysRoot, "usr/bin/sudo")
	if got, ok := resolveTrustedBinaryAt(sysRoot, uid, "/usr/bin/sudo", "/bin/sudo"); !ok || got != wantSudo {
		t.Fatalf("alternatives sudo resolution = (%q, %v), want (%q, true)", got, ok, wantSudo)
	}

	// Reject relative, uncleaned, directory-target, symlink-loop, and group-writable chains.
	if err := os.Symlink("/usr/bin/", filepath.Join(sysRoot, "usr/bin/dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop-b", filepath.Join(sysRoot, "usr/bin/loop-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop-a", filepath.Join(sysRoot, "usr/bin/loop-b")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"bin/sh", "/usr/../bin/sh", "/usr/bin", "/usr/bin/dirlink", "/usr/bin/loop-a"} {
		if got, ok := resolveTrustedBinaryAt(sysRoot, uid, bad); ok {
			t.Fatalf("expected %q to be rejected, got %q", bad, got)
		}
	}

	if err := os.Chmod(filepath.Join(sysRoot, "etc/alternatives"), 0o775); err != nil {
		t.Fatal(err)
	}
	if got, ok := resolveTrustedBinaryAt(sysRoot, uid, "/usr/bin/sudo"); ok {
		t.Fatalf("expected group-writable intermediate symlink directory to be rejected, got %q", got)
	}
}
