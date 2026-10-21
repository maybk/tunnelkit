//go:build darwin

package netdev

import (
	"golang.zx2c4.com/wireguard/tun"

	"tunnelkit/fortivpn"
)

// Open creates a macOS utun interface with the requested MTU.
func Open(mtu int) (Device, error) {
	if mtu <= 0 {
		mtu = 1400
	}
	dev, err := tun.CreateTUN("utun", mtu)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeDeviceFailed, "failed to create macOS utun device", err)
	}
	return wrapTUN(dev, "utun", mtu)
}
