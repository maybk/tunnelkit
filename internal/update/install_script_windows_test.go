//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWindowsInstallerReplacesTogetherOrRestoresOriginals(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "missing-component"}[complete], func(t *testing.T) {
			stage, target := t.TempDir(), t.TempDir()
			names := []string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"}
			for i, name := range names {
				if err := os.WriteFile(filepath.Join(target, name), []byte("old"), 0o755); err != nil {
					t.Fatal(err)
				}
				if complete || i == 0 {
					if err := os.WriteFile(filepath.Join(stage, name), []byte("new"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			script := filepath.Join(stage, "apply.cmd")
			content := buildWindowsFileUpdate(stage, target, names, filepath.Join(stage, "ok"), filepath.Join(stage, "error"))
			if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("cmd.exe", "/d", "/c", script).CombinedOutput()
			if complete != (err == nil) {
				t.Fatalf("installer: %v (%s)", err, out)
			}
			for _, name := range names {
				got, err := os.ReadFile(filepath.Join(target, name))
				want := "old"
				if complete {
					want = "new"
				}
				if err != nil || string(got) != want {
					t.Fatalf("%s: %q (%v), want %s", name, got, err, want)
				}
			}
		})
	}
}
