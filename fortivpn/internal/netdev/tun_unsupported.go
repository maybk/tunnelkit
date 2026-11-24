//go:build !darwin && !linux && !windows

package netdev

import "tunnelkit/fortivpn"

// Open returns CodeUnsupportedPlatform on unsupported targets.
func Open(_ int) (Device, error) {
	return nil, fortivpn.NewError(fortivpn.CodeUnsupportedPlatform, "TUN device is not supported on this platform")
}
