//go:build !darwin && !linux && !windows

package helperclient

import "errors"

func processPresent(int) (bool, error) {
	return false, errors.New("helper process observation is unsupported")
}
