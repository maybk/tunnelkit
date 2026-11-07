//go:build !unix && !windows

package update

func processAlive(int) (bool, error) {
	return false, nil
}
