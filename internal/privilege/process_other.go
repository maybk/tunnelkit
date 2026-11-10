//go:build !darwin && !linux && !windows

package privilege

func isProcessAlive(int) (bool, error) {
	return false, unsupported()
}
