//go:build !darwin && !linux && !windows

package privilege

import (
	"context"
	"errors"
	"os"
)

func unsupported() error {
	return errors.New("privileged helper is unsupported on this platform")
}

func ExecutablePath() (string, error)            { return "", unsupported() }
func stateRoot() (string, error)                 { return "", unsupported() }
func validateOwner(string) error                 { return unsupported() }
func protectDirectory(string) error              { return unsupported() }
func verifyInstalled(string) error               { return unsupported() }
func protectComponent(string, os.FileMode) error { return unsupported() }
func componentPairs(a, b string) [][2]string     { return [][2]string{{a, b}} }
func ValidateEndpoint(string, string) error      { return unsupported() }
func PrepareEndpoint(string) error               { return unsupported() }
func Install(string) error                       { return unsupported() }
func Uninstall() error                           { return unsupported() }
func serviceCurrent() (bool, error)              { return false, unsupported() }
func InstallService(string) error                { return unsupported() }
func UninstallService() error                    { return unsupported() }
func elevateInstall(context.Context, string) error {
	return unsupported()
}
