//go:build windows

package credentials

import (
	"errors"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

func sealKeyBytes(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > math.MaxUint32 {
		return nil, errors.New("key material exceeds DPAPI size limit")
	}
	inBlob := windows.DataBlob{
		Size: uint32(len(raw)), // #nosec G115 -- bounded to math.MaxUint32 above.
		Data: &raw[0],
	}
	var outBlob windows.DataBlob
	if err := windows.CryptProtectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &outBlob); err != nil {
		return nil, err
	}
	defer func() {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data))) // #nosec G103 -- releases Windows DPAPI output buffer.
	}()

	out := make([]byte, outBlob.Size)
	copy(out, unsafe.Slice(outBlob.Data, outBlob.Size)) // #nosec G103 -- copies DPAPI output buffer allocated by Windows.
	return out, nil
}

func unsealKeyBytes(sealed []byte) ([]byte, error) {
	if len(sealed) == 0 {
		return nil, nil
	}
	if len(sealed) > math.MaxUint32 {
		return nil, errors.New("sealed key exceeds DPAPI size limit")
	}
	inBlob := windows.DataBlob{
		Size: uint32(len(sealed)), // #nosec G115 -- bounded to math.MaxUint32 above.
		Data: &sealed[0],
	}
	var outBlob windows.DataBlob
	if err := windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &outBlob); err != nil {
		return nil, err
	}
	defer func() {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data))) // #nosec G103 -- releases Windows DPAPI output buffer.
	}()

	out := make([]byte, outBlob.Size)
	copy(out, unsafe.Slice(outBlob.Data, outBlob.Size)) // #nosec G103 -- copies DPAPI output buffer allocated by Windows.
	return out, nil
}
