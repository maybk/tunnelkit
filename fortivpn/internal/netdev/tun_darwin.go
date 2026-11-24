//go:build darwin

package netdev

import (
	"os"

	"golang.org/x/sys/unix"
	"golang.zx2c4.com/wireguard/tun"

	"tunnelkit/fortivpn"
)

const (
	darwinSysprotoControl          = 2
	darwinUtunOptMaxPendingPackets = 16 // <net/if_utun.h> UTUN_OPT_MAX_PENDING_PACKETS
	darwinUtunHeaderLen            = 4
	darwinUtunRecvBufTarget        = 8 << 20
	darwinUtunRecvBufMin           = 1 << 20
	darwinUtunRecvBufDefault       = 512 << 10
	darwinUtunMaxPendingCap        = 64
	darwinUtunMTUBaseline          = 1500
)

// Open creates a macOS utun interface with the requested MTU and tunes its
// kernel receive buffer and pending-packet limit to avoid burst ENOBUFS drops.
func Open(mtu int) (Device, error) {
	if mtu <= 0 {
		mtu = 1400
	}
	dev, err := tun.CreateTUN("utun", mtu)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeDeviceFailed, "failed to create macOS utun device", err)
	}
	tuneDarwinUTUN(dev.File(), mtu)
	return wrapTUN(dev, "utun", mtu)
}

// tuneDarwinUTUN enlarges the utun control socket SO_RCVBUF via SyscallConn
// (preserving non-blocking Go netpoller state) and raises UTUN_OPT_MAX_PENDING_PACKETS
// within 7/8 of SO_RCVBUF so XNU applies backpressure instead of ENOBUFS drops.
func tuneDarwinUTUN(f *os.File, mtu int) {
	if f == nil {
		return
	}
	raw, err := f.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		actualBuf := raiseDarwinSocketRecvBuf(int(fd))
		pending := computeUTUNMaxPendingPackets(actualBuf, mtu)
		_ = unix.SetsockoptInt(int(fd), darwinSysprotoControl, darwinUtunOptMaxPendingPackets, pending)
	})
}

func raiseDarwinSocketRecvBuf(fd int) int {
	for size := darwinUtunRecvBufTarget; size >= darwinUtunRecvBufMin; size /= 2 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, size); err == nil {
			break
		}
	}
	current, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF)
	if err != nil || current <= 0 {
		return darwinUtunRecvBufDefault
	}
	return current
}

func computeUTUNMaxPendingPackets(recvBuf, mtu int) int {
	if recvBuf <= 0 {
		recvBuf = darwinUtunRecvBufDefault
	}
	effectiveMTU := max(mtu, darwinUtunMTUBaseline)
	pending := (recvBuf / 8 * 7) / (effectiveMTU + darwinUtunHeaderLen)
	return max(min(pending, darwinUtunMaxPendingCap), 1)
}
