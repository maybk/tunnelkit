// Command tunnelkit-helper is the privileged single-session helper daemon for TunnelKit.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tunnelkit/internal/helper"
)

// Version is injected at build time via -ldflags "-X main.Version=...".
var Version = "0.1.0"

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "Usage: tunnelkit-helper serve --endpoint <path> --owner <uid|sid> --token-sha256 <hex> [--log <path>] [--journal <path>]")
		os.Exit(2)
	}

	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	endpoint := fs.String("endpoint", "", "IPC socket or named pipe endpoint")
	owner := fs.String("owner", "", "authorized peer UID (macOS) or SID (Windows)")
	uidAlias := fs.String("uid", "", "alias for --owner on macOS")
	sidAlias := fs.String("sid", "", "alias for --owner on Windows")
	tokenSHA256 := fs.String("token-sha256", "", "SHA-256 hex digest of the session token")
	logPath := fs.String("log", "", "session log file path")
	journalPath := fs.String("journal", "", "network rollback journal path")
	_ = fs.Parse(os.Args[2:])

	ownerID := *owner
	if ownerID == "" {
		ownerID = *uidAlias
	}
	if ownerID == "" {
		ownerID = *sidAlias
	}

	if *endpoint == "" || ownerID == "" || *tokenSHA256 == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --endpoint, --owner (--uid/--sid), and --token-sha256 are required")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := helper.Serve(ctx, helper.Options{
		Endpoint:    *endpoint,
		OwnerID:     ownerID,
		TokenSHA256: *tokenSHA256,
		LogPath:     *logPath,
		JournalPath: *journalPath,
		Version:     Version,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
