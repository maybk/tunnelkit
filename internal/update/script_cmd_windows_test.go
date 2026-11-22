//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsInstallerReplacesTogetherOrRestoresOriginals(t *testing.T) {
	for _, tc := range []struct {
		name      string
		complete  bool
		wrongHash bool
	}{
		{name: "complete-verified", complete: true},
		{name: "missing-component", complete: false},
		{name: "checksum-mismatch", complete: true, wrongHash: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, target := t.TempDir(), t.TempDir()
			names := []string{"tunnelkit.exe", "tunnelkit-helper.exe", "wintun.dll"}
			sums := make(map[string]string, len(names))
			for i, name := range names {
				if err := os.WriteFile(filepath.Join(target, name), []byte("old"), 0o755); err != nil {
					t.Fatal(err)
				}
				if tc.complete || i == 0 {
					p := filepath.Join(stage, name)
					if err := os.WriteFile(p, []byte("new"), 0o755); err != nil {
						t.Fatal(err)
					}
					sum, err := fileSHA256Hex(p)
					if err != nil {
						t.Fatal(err)
					}
					sums[name] = sum
				}
			}
			if tc.wrongHash {
				sums[names[len(names)-1]] = strings.Repeat("0", 64)
			}
			script := filepath.Join(stage, "apply.cmd")
			content := buildWindowsFileUpdateWithHelperSync(stage, target, names, filepath.Join(stage, "ok"), filepath.Join(stage, "error"), "", sums)
			if err := os.WriteFile(script, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(windowsCmdExePath(), "/d", "/c", script).CombinedOutput()
			wantSuccess := tc.complete && !tc.wrongHash
			if wantSuccess != (err == nil) {
				t.Fatalf("installer: %v (%s)", err, out)
			}
			for _, name := range names {
				got, err := os.ReadFile(filepath.Join(target, name))
				want := "old"
				if wantSuccess {
					want = "new"
				}
				if err != nil || string(got) != want {
					t.Fatalf("%s: %q (%v), want %s", name, got, err, want)
				}
				if _, err := os.Stat(filepath.Join(target, name+".new")); !os.IsNotExist(err) {
					t.Fatalf("%s.new was not cleaned up (err=%v)", name, err)
				}
			}
		})
	}
}
