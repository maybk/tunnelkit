//go:build windows

package helperclient

import (
	"context"
	"os"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/privilege"
)

func TestSmokeLaunchInstalledWindowsHelper(t *testing.T) {
	src := os.Getenv("TUNNELKIT_SMOKE_HELPER_SOURCE")
	if src == "" {
		t.Skip("TUNNELKIT_SMOKE_HELPER_SOURCE not set")
	}
	ok, err := privilege.Current(src)
	if err != nil || !ok {
		t.Fatalf("expected installed Windows helper to be current before smoke launch (ok=%v, err=%v)", ok, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, ticket, err := Launch(ctx, LaunchRequest{
		HelperPath:   src,
		ProfileID:    "smoke-win",
		ProfileName:  "Windows Smoke",
		RecoveryOnly: true,
	})
	if err != nil {
		t.Fatalf("Launch(RecoveryOnly) via Windows SCM failed: %v", err)
	}
	defer client.Close()

	if ticket.PID <= 0 || ticket.Endpoint == "" {
		t.Fatalf("unexpected smoke ticket: %+v", ticket)
	}

	var sawDisconnected bool
	for msg := range client.Events() {
		if msg.Event != nil && msg.Event.Phase == fortivpn.PhaseDisconnected {
			if msg.Event.Err != nil {
				t.Fatalf("smoke recovery session returned error event: %v", msg.Event.Err)
			}
			sawDisconnected = true
			break
		}
	}
	if !sawDisconnected {
		t.Fatal("expected PhaseDisconnected event from Windows helper recovery session")
	}

	if err := client.WaitForExit(ctx); err != nil {
		t.Fatalf("WaitForExit after Windows helper session failed: %v", err)
	}
}
