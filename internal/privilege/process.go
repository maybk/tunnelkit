package privilege

// IsProcessAlive reports whether a positive PID is still present. Unknown probe
// failures are returned to the caller and must not be treated as process exit.
func IsProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	return isProcessAlive(pid)
}
