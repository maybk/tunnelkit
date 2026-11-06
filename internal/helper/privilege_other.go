//go:build !darwin && !linux && !windows

package helper

// EnsureRootIdentity is a no-op on unsupported platforms.
func EnsureRootIdentity() error {
	return nil
}

// RunIfWindowsService returns false on non-Windows platforms.
func RunIfWindowsService(_ string) (bool, error) {
	return false, nil
}
