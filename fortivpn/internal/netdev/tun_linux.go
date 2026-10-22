//go:build linux

package netdev

import (
	"golang.zx2c4.com/wireguard/tun"

	"tunnelkit/fortivpn"
)

// Open creates a Linux TUN interface (/dev/net/tun) with the requested MTU.
func Open(mtu int) (Device, error) {
	if mtu <= 0 {
		mtu = 1400
	}
	dev, err := tun.CreateTUN("forti%d", mtu)
	if err != nil {
		dev, err = tun.CreateTUN("forti0", mtu)
	}
	if err != nil {
		// Fall back to kernel-assigned tun%d if forti* cannot be created.
		dev, err = tun.CreateTUN("tun%d", mtu)
		if err != nil {
			return nil, fortivpn.WrapError(fortivpn.CodeDeviceFailed, "failed to create Linux TUN device", err)
		}
	}
	return wrapTUN(dev, "forti0", mtu)
}
