// Package netdev provides a minimal cross-platform TUN device wrapper over
// golang.zx2c4.com/wireguard/tun with in-place headroom support.
package netdev

import (
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

// Headroom is the leading byte offset reserved before the Layer-3 IPv4 packet
// so kernel TUN headers (4B BSD utun or 10B Linux virtio_net_hdr) and
// Fortinet/PPP framing (6B 0x5050 + 4B PPP = 10B) can be written in place.
const Headroom = 16

const packetOffset = Headroom

// Device represents an opened virtual network interface.
type Device interface {
	Name() string
	MTU() int
	ReadPacket(buf []byte) (int, error)
	WritePacket(pkt []byte) error
	Close() error
}

type tunDevice struct {
	dev       tun.Device
	name      string
	mtu       int
	readMu    sync.Mutex
	readBuf   []byte
	readBufs  [][]byte
	readSizes []int
	writeMu   sync.Mutex
	writeBuf  []byte
	writeBufs [][]byte
	closeOnce sync.Once
	closeErr  error
}

func wrapTUN(dev tun.Device, fallbackName string, mtu int) (Device, error) {
	if mtu <= 0 {
		mtu = 1400
	}
	name, err := dev.Name()
	if err != nil || name == "" {
		name = fallbackName
	}
	maxBuf := packetOffset + 65535
	td := &tunDevice{
		dev:       dev,
		name:      name,
		mtu:       mtu,
		readBuf:   make([]byte, maxBuf),
		readBufs:  make([][]byte, 1),
		readSizes: make([]int, 1),
		writeBuf:  make([]byte, maxBuf),
		writeBufs: make([][]byte, 1),
	}
	// Drain tun.Events() so wireguard-go's event channel never blocks.
	go func() {
		for range dev.Events() {
		}
	}()
	return td, nil
}

func (t *tunDevice) Name() string { return t.name }
func (t *tunDevice) MTU() int {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.mtu
}

func (t *tunDevice) SetMTU(mtu int) {
	if mtu <= 0 {
		return
	}
	t.writeMu.Lock()
	t.mtu = mtu
	t.writeMu.Unlock()
}

// ReadPacketAt reads an outbound IP packet directly into buf[offset : offset+n]
// without intermediate buffer copying when offset >= Headroom.
func (t *tunDevice) ReadPacketAt(buf []byte, offset int) (int, error) {
	if offset < packetOffset || len(buf) <= offset {
		if len(buf) <= offset {
			return 0, nil
		}
		return t.ReadPacket(buf[offset:])
	}
	t.readMu.Lock()
	defer t.readMu.Unlock()

	t.readBufs[0] = buf
	t.readSizes[0] = 0
	n, err := t.dev.Read(t.readBufs, t.readSizes, offset)
	if err != nil {
		return 0, err
	}
	if n == 0 || t.readSizes[0] <= 0 {
		return 0, nil
	}
	return t.readSizes[0], nil
}

func (t *tunDevice) ReadPacket(buf []byte) (int, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	t.readBufs[0] = t.readBuf
	t.readSizes[0] = 0
	n, err := t.dev.Read(t.readBufs, t.readSizes, packetOffset)
	if err != nil {
		return 0, err
	}
	if n == 0 || t.readSizes[0] <= 0 {
		return 0, nil
	}
	size := t.readSizes[0]
	copied := copy(buf, t.readBuf[packetOffset:packetOffset+size])
	return copied, nil
}

// WritePacketAt writes an inbound IP packet located at buf[offset:] directly to
// the kernel TUN interface using buf[:offset] as kernel header scratch space.
func (t *tunDevice) WritePacketAt(buf []byte, offset int) error {
	if len(buf) <= offset {
		return nil
	}
	if offset < packetOffset {
		return t.WritePacket(buf[offset:])
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	t.writeBufs[0] = buf
	_, err := t.dev.Write(t.writeBufs, offset)
	return err
}

func (t *tunDevice) WritePacket(pkt []byte) error {
	if len(pkt) == 0 {
		return nil
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	end := packetOffset + len(pkt)
	if end > len(t.writeBuf) {
		t.writeBuf = make([]byte, end)
	}
	copy(t.writeBuf[packetOffset:end], pkt)
	t.writeBufs[0] = t.writeBuf[:end]
	_, err := t.dev.Write(t.writeBufs, packetOffset)
	return err
}

func (t *tunDevice) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = t.dev.Close()
	})
	return t.closeErr
}
