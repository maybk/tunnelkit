package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHelperVersionCommandDoesNotRequireRoot(t *testing.T) {
	if !isVersionArg([]string{"version"}) || !isVersionArg([]string{"--version"}) || !isVersionArg([]string{"-version"}) {
		t.Fatal("expected version flags to match")
	}
	for _, bad := range [][]string{nil, {}, {"serve"}, {"--version", "extra"}, {"install"}} {
		if isVersionArg(bad) {
			t.Fatalf("unexpected version match for %v", bad)
		}
	}

	bin := filepath.Join(t.TempDir(), "tunnelkit-helper")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags", "-X main.Version=9.8.7-test", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build helper: %v (%s)", err, out)
	}

	for _, flag := range []string{"--version", "version"} {
		out, err := exec.Command(bin, flag).CombinedOutput()
		if err != nil {
			t.Fatalf("helper %s failed: %v (%s)", flag, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "9.8.7-test" {
			t.Fatalf("helper %s = %q, want %q", flag, got, "9.8.7-test")
		}
	}
}
