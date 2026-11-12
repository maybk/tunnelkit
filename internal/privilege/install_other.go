//go:build !darwin && !linux && !windows

package privilege

import (
	"context"
	"os"
)

func protectDirectory(string) error              { return unsupported() }
func inspectInstalled(string) (string, error)    { return "", unsupported() }
func protectComponent(string, os.FileMode) error { return unsupported() }
func componentPairs(a, b string) [][2]string     { return [][2]string{{a, b}} }
func Install(string) error                       { return unsupported() }
func Uninstall() error                           { return unsupported() }
func elevateInstall(context.Context, string) error {
	return unsupported()
}
