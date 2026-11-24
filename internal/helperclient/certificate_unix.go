//go:build unix

package helperclient

import (
	"os"
	"syscall"
)

func openCertificateFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
