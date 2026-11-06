package helper

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// ParseServeArgs parses command-line arguments (or Windows SCM StartService
// arguments) into helper Options. A leading "serve" subcommand token is optional.
func ParseServeArgs(args []string, version string) (Options, error) {
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	endpoint := fs.String("endpoint", "", "IPC socket or named pipe endpoint")
	owner := fs.String("owner", "", "authorized peer UID (POSIX) or SID (Windows)")
	uidAlias := fs.String("uid", "", "alias for --owner on POSIX")
	sidAlias := fs.String("sid", "", "alias for --owner on Windows")
	tokenSHA256 := fs.String("token-sha256", "", "SHA-256 hex digest of the session token")

	if err := fs.Parse(args); err != nil {
		return Options{}, fmt.Errorf("parse serve flags: %w", err)
	}

	if fs.NArg() != 0 {
		return Options{}, errors.New("unexpected serve arguments")
	}
	ownerID := strings.TrimSpace(*owner)
	if ownerID == "" {
		ownerID = strings.TrimSpace(*uidAlias)
	}
	if ownerID == "" {
		ownerID = strings.TrimSpace(*sidAlias)
	}

	ep := strings.TrimSpace(*endpoint)
	tokenHash := strings.TrimSpace(*tokenSHA256)
	if ep == "" || ownerID == "" || tokenHash == "" {
		return Options{}, errors.New("--endpoint, --owner (--uid/--sid), and --token-sha256 are required")
	}

	if hash, err := hex.DecodeString(tokenHash); err != nil || len(hash) != 32 {
		return Options{}, errors.New("invalid session token digest")
	}
	return Options{
		Endpoint:    ep,
		OwnerID:     ownerID,
		TokenSHA256: tokenHash,
		Version:     version,
	}, nil
}
