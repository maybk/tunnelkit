//go:build darwin || linux

package privilege

import "testing"

func TestShellQuoteCommand(t *testing.T) {
	got := shellQuoteCommand("/Applications/TunnelKit App/helper", []string{"install"})
	if got != "'/Applications/TunnelKit App/helper' 'install'" {
		t.Fatalf("quoted command: %s", got)
	}
}
