//go:build windows

package netdev

import (
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"

	"tunnelkit/fortivpn"
)

// AdapterName is the fixed Wintun adapter name used on Windows.
const AdapterName = "TunnelKit"

// Fixed GUID so any stale adapter from a crashed helper is reused/replaced cleanly.
var tunnelKitGUID = windows.GUID{
	Data1: 0x7a4c8e21,
	Data2: 0x5b90,
	Data3: 0x4d3f,
	Data4: [8]byte{0x9e, 0x12, 0x8c, 0x4b, 0x6f, 0x0a, 0x1d, 0x2e},
}

// Open creates or reuses the TunnelKit Wintun adapter with a fixed GUID.
func Open(mtu int) (Device, error) {
	if mtu <= 0 {
		mtu = 1400
	}
	// Restrict DLL search path to system32 + application directory to prevent DLL hijacking.
	_ = windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32 | windows.LOAD_LIBRARY_SEARCH_APPLICATION_DIR)

	tun.WintunTunnelType = "TunnelKit"
	dev, err := tun.CreateTUNWithRequestedGUID(AdapterName, &tunnelKitGUID, mtu)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeDeviceFailed, "failed to create Windows Wintun adapter", err)
	}
	return wrapTUN(dev, AdapterName, mtu)
}
