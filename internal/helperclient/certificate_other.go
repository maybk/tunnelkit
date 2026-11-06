//go:build !darwin && !linux && !windows

package helperclient

import "os"

func openCertificateFile(path string) (*os.File, error) {
	return os.Open(path)
}
