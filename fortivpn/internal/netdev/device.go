// Package netdev provides a minimal cross-platform TUN device wrapper over
// golang.zx2c4.com/wireguard/tun.
package netdev

import (
	"sync"

	"golang.zx2c4.com/wireguard/tun"
)

const packetOffset = 16

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
func (t *tunDevice) MTU() int     { return t.mtu }

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
