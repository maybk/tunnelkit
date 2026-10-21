package netcfg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type cmdRunner func(ctx context.Context, stdin string, name string, args ...string) (string, error)

func defaultCmdRunner(ctx context.Context, stdin string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(out.String())
		if msg != "" {
			return out.String(), fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, msg)
		}
		return out.String(), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out.String(), nil
}

func isRouteAlreadyExists(out string, err error) bool {
	combined := strings.ToLower(out + " " + err.Error())
	return strings.Contains(combined, "file exists") || strings.Contains(combined, "already in table")
}

func isRouteNotInTable(out string, err error) bool {
	combined := strings.ToLower(out + " " + err.Error())
	return strings.Contains(combined, "not in table") ||
		strings.Contains(combined, "no such process") ||
		strings.Contains(combined, "no such device") ||
		strings.Contains(combined, "cannot find device")
}
