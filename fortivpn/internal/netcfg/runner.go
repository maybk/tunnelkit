package netcfg

import (
	"context"
	"strings"
)

type cmdRunner func(ctx context.Context, stdin string, name string, args ...string) (string, error)

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
