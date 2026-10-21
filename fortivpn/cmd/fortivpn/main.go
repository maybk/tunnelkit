// Command fortivpn is a standalone CLI for the pure-Go Fortinet SSL-VPN kernel.
//
// Usage:
//
//	fortivpn connect -c <conf> [-v] [--journal <path>]
//	fortivpn check   -c <conf> [-v] [--dump-config <path>]
//	fortivpn recover --journal <path>
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/engine"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "connect":
		err = runConnect(ctx, args)
	case "check":
		err = runCheck(ctx, args)
	case "recover":
		err = runRecover(ctx, args)
	case "-h", "--help", "help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  fortivpn connect -c <config-file> [-v] [--journal <path>]
  fortivpn check   -c <config-file> [-v] [--dump-config <path>]
  fortivpn recover --journal <path>`)
}

func runConnect(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	confPath := fs.String("c", "", "path to openfortivpn-compatible config file")
	verbose := fs.Bool("v", false, "enable debug logging")
	journalPath := fs.String("journal", "", "optional path to network rollback journal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *confPath == "" {
		return fmt.Errorf("-c <config-file> is required")
	}

	logger := newLogger(*verbose)
	cfg, err := loadConfig(*confPath, logger)
	if err != nil {
		return err
	}

	opts := []engine.Option{engine.WithLogger(logger)}
	if *journalPath != "" {
		opts = append(opts, engine.WithJournal(*journalPath))
	}
	return engine.Run(ctx, cfg, &cliHandler{log: logger}, opts...)
}

func runCheck(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	confPath := fs.String("c", "", "path to openfortivpn-compatible config file")
	verbose := fs.Bool("v", false, "enable debug logging")
	dumpPath := fs.String("dump-config", "", "optional file path to write fortisslvpn_xml response")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *confPath == "" {
		return fmt.Errorf("-c <config-file> is required")
	}

	logger := newLogger(*verbose)
	cfg, err := loadConfig(*confPath, logger)
	if err != nil {
		return err
	}

	opts := []engine.Option{engine.WithLogger(logger)}
	if *dumpPath != "" {
		target := *dumpPath
		opts = append(opts, engine.WithConfigXMLHook(func(raw []byte) {
			_ = os.WriteFile(target, raw, 0o600)
		}))
	}

	info, err := engine.Check(ctx, cfg, &cliHandler{log: logger}, opts...)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}

func runRecover(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	journalPath := fs.String("journal", "", "path to network rollback journal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *journalPath == "" {
		return fmt.Errorf("--journal <path> is required")
	}
	return engine.Recover(ctx, *journalPath)
}

func loadConfig(path string, log *slog.Logger) (fortivpn.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fortivpn.Config{}, err
	}
	cfg, warnings, err := fortivpn.ParseConfigText(string(data))
	if err != nil {
		return fortivpn.Config{}, err
	}
	for _, w := range warnings {
		log.Warn(w.String())
	}
	return cfg, nil
}

func newLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

type cliHandler struct {
	log *slog.Logger
}

func (h *cliHandler) OnEvent(ev fortivpn.Event) {
	if ev.Err != nil {
		h.log.Error("engine error", "phase", ev.Phase, "code", ev.Err.Code, "detail", ev.Err.Detail, "fingerprint", ev.Err.Fingerprint)
	}
}

func (h *cliHandler) Prompt(ctx context.Context, p fortivpn.Prompt) (string, error) {
	switch p.Kind {
	case fortivpn.PromptSAML:
		fmt.Fprintf(os.Stderr, "Complete SAML authentication in your browser:\n  %s\n", p.URL)
		return "", nil
	case fortivpn.PromptFTMPush:
		fmt.Fprintf(os.Stderr, "%s (press Enter for push or type OTP code): ", p.Message)
	default:
		fmt.Fprintf(os.Stderr, "%s: ", p.Message)
	}

	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		r := bufio.NewReader(os.Stdin)
		line, err := r.ReadString('\n')
		if err != nil && len(line) == 0 {
			errCh <- err
			return
		}
		lineCh <- strings.TrimSpace(line)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case line := <-lineCh:
		return line, nil
	}
}
