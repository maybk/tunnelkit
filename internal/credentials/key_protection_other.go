//go:build !windows

package credentials

func sealKeyBytes(raw []byte) ([]byte, error) {
	return raw, nil
}

func unsealKeyBytes(sealed []byte) ([]byte, error) {
	return sealed, nil
}
