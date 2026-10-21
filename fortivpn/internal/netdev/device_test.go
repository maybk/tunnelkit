package netdev

import (
	"bytes"
	"os"
	"sync/atomic"
	"testing"

	"golang.zx2c4.com/wireguard/tun"
)

type fakeWireGuardTUN struct {
	name      string
	events    chan tun.Event
	lastWrite []byte
	readPkt   []byte
	closed    atomic.Int32
}

func newFakeWireGuardTUN(name string, readPkt []byte) *fakeWireGuardTUN {
	return &fakeWireGuardTUN{
		name:    name,
		events:  make(chan tun.Event, 4),
		readPkt: readPkt,
	}
}

func (f *fakeWireGuardTUN) File() *os.File { return nil }
func (f *fakeWireGuardTUN) MTU() (int, error) {
	return 1400, nil
}
func (f *fakeWireGuardTUN) Name() (string, error) {
	return f.name, nil
}
func (f *fakeWireGuardTUN) Events() <-chan tun.Event {
	return f.events
}
func (f *fakeWireGuardTUN) BatchSize() int { return 1 }

func (f *fakeWireGuardTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(f.readPkt) == 0 {
		return 0, nil
	}
	n := copy(bufs[0][offset:], f.readPkt)
	sizes[0] = n
	return 1, nil
}

func (f *fakeWireGuardTUN) Write(bufs [][]byte, offset int) (int, error) {
	f.lastWrite = append([]byte(nil), bufs[0][offset:]...)
	return 1, nil
}

func (f *fakeWireGuardTUN) Flush() error { return nil }

func (f *fakeWireGuardTUN) Close() error {
	if f.closed.Add(1) == 1 {
		close(f.events)
	}
	return nil
}

func TestWrapTUNReadWriteAndIdempotentClose(t *testing.T) {
	wantRead := []byte{0x45, 0x00, 0x00, 0x14, 1, 2, 3, 4}
	raw := newFakeWireGuardTUN("", wantRead)

	dev, err := wrapTUN(raw, "utun77", 0)
	if err != nil {
		t.Fatalf("wrapTUN failed: %v", err)
	}
	if dev.Name() != "utun77" {
		t.Fatalf("Name() = %q, want fallback utun77", dev.Name())
	}
	if dev.MTU() != 1400 {
		t.Fatalf("MTU() = %d, want default 1400", dev.MTU())
	}

	buf := make([]byte, 64)
	n, err := dev.ReadPacket(buf)
	if err != nil {
		t.Fatalf("ReadPacket failed: %v", err)
	}
	if !bytes.Equal(buf[:n], wantRead) {
		t.Fatalf("ReadPacket = %x, want %x", buf[:n], wantRead)
	}

	if err := dev.WritePacket(nil); err != nil {
		t.Fatalf("WritePacket(nil) failed: %v", err)
	}
	wantWrite := []byte{0x45, 0x00, 0x00, 0x20, 9, 8, 7, 6}
	if err := dev.WritePacket(wantWrite); err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}
	if !bytes.Equal(raw.lastWrite, wantWrite) {
		t.Fatalf("underlying Write got %x, want %x", raw.lastWrite, wantWrite)
	}

	if err := dev.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := dev.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if got := raw.closed.Load(); got != 1 {
		t.Fatalf("underlying Close count = %d, want 1", got)
	}
}
