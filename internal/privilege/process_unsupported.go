//go:build !unix && !windows

package privilege

func isProcessAlive(int) (bool, error) {
	return false, unsupported()
}
