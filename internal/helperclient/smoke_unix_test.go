//go:build darwin || linux

package helperclient

import (
	"context"
	"os"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/privilege"
)

func TestSmokeLaunchInstalledUnixHelper(t *testing.T) {
	src := os.Getenv("TUNNELKIT_SMOKE_HELPER_SOURCE")
	if src == "" {
		t.Skip("TUNNELKIT_SMOKE_HELPER_SOURCE not set")
	}
	ok, err := privilege.Current(src)
	if err != nil || !ok {
		t.Fatalf("expected installed Unix helper to be current before smoke launch (ok=%v, err=%v)", ok, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, ticket, err := Launch(ctx, LaunchRequest{
		HelperPath:   src,
		ProfileID:    "smoke-unix",
		ProfileName:  "Unix Smoke",
		RecoveryOnly: true,
	})
	if err != nil {
		t.Fatalf("Launch(RecoveryOnly) via installed setuid helper failed: %v", err)
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
		t.Fatal("expected PhaseDisconnected event from Unix helper recovery session")
	}

	if err := client.WaitForExit(ctx); err != nil {
		t.Fatalf("WaitForExit after Unix helper session failed: %v", err)
	}
}
