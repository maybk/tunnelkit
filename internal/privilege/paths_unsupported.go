//go:build !darwin && !linux && !windows

package privilege

import "errors"

func unsupported() error {
	return errors.New("privileged helper is unsupported on this platform")
}

func ExecutablePath() (string, error)       { return "", unsupported() }
func stateRoot() (string, error)            { return "", unsupported() }
func validateOwner(string) error            { return unsupported() }
func ValidateEndpoint(string, string) error { return unsupported() }
func PrepareEndpoint(string) error          { return unsupported() }
