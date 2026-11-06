// Command tunnelkit-helper is the privileged single-session helper daemon for TunnelKit.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tunnelkit/internal/helper"
	"tunnelkit/internal/privilege"
)

// Version is injected at build time via -ldflags "-X main.Version=...".
var Version = "0.1.0"

func main() {
	if err := helper.EnsureRootIdentity(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if ranAsSvc, err := helper.RunIfWindowsService(Version); ranAsSvc {
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		opts, err := helper.ParseServeArgs(os.Args[2:], Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(2)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := helper.Serve(ctx, opts); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}

	case "install":
		exe, err := os.Executable()
		if err == nil {
			err = privilege.Install(exe)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}

	case "uninstall":
		if err := privilege.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}

	case "install-service":
		if err := privilege.InstallService(""); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}

	case "uninstall-service":
		if err := privilege.UninstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}

	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  tunnelkit-helper serve --endpoint <path> --owner <uid|sid> --token-sha256 <hex>")
	fmt.Fprintln(os.Stderr, "  tunnelkit-helper install (POSIX) | install-service (Windows)")
	fmt.Fprintln(os.Stderr, "  tunnelkit-helper uninstall (POSIX) | uninstall-service (Windows)")
}
