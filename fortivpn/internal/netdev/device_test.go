package netdev

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/tun"
)

type fakeWireGuardTUN struct {
	name             string
	mtu              atomic.Int64
	batchSize        int
	events           chan tun.Event
	readBatches      [][][]byte
	readErrs         []error
	readIdx          int
	written          [][]byte
	writeReturnBytes bool
	writeErr         error
	closed           atomic.Int32
}

func newFakeWireGuardTUN(name string, readPkt []byte) *fakeWireGuardTUN {
	f := &fakeWireGuardTUN{
		name:      name,
		batchSize: 1,
		events:    make(chan tun.Event, 4),
	}
	f.mtu.Store(1400)
	if len(readPkt) > 0 {
		f.readBatches = [][][]byte{{readPkt}}
	}
	return f
}

func (f *fakeWireGuardTUN) File() *os.File        { return nil }
func (f *fakeWireGuardTUN) MTU() (int, error)     { return int(f.mtu.Load()), nil }
func (f *fakeWireGuardTUN) Name() (string, error) { return f.name, nil }
func (f *fakeWireGuardTUN) Events() <-chan tun.Event {
	return f.events
}
func (f *fakeWireGuardTUN) BatchSize() int { return f.batchSize }

func (f *fakeWireGuardTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(f.readBatches) == 0 {
		return 0, nil
	}
	idx := f.readIdx
	if idx >= len(f.readBatches) {
		if len(f.readBatches) == 1 && len(f.readErrs) == 0 {
			idx = 0
		} else {
			return 0, io.EOF
		}
	} else {
		f.readIdx++
	}
	batch := f.readBatches[idx]
	if len(bufs) < len(batch) {
		return 0, errors.New("ErrTooManySegments")
	}
	for i, pkt := range batch {
		sizes[i] = copy(bufs[i][offset:], pkt)
	}
	var err error
	if idx < len(f.readErrs) {
		err = f.readErrs[idx]
	}
	return len(batch), err
}

func (f *fakeWireGuardTUN) Write(bufs [][]byte, offset int) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	totalBytes := 0
	for _, b := range bufs {
		pkt := append([]byte(nil), b[offset:]...)
		f.written = append(f.written, pkt)
		totalBytes += len(pkt)
	}
	if f.writeReturnBytes {
		return totalBytes, nil
	}
	return len(bufs), nil
}

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

	td := dev.(*tunDevice)
	headBuf := make([]byte, Headroom+64)
	n, err = td.ReadPacketAt(headBuf, Headroom)
	if err != nil || !bytes.Equal(headBuf[Headroom:Headroom+n], wantRead) {
		t.Fatalf("ReadPacketAt = %x (err=%v), want %x", headBuf[Headroom:Headroom+n], err, wantRead)
	}

	wantWrite := []byte{0x45, 0x00, 0x00, 0x20, 9, 8, 7, 6}
	writeBuf := make([]byte, Headroom+len(wantWrite))
	copy(writeBuf[Headroom:], wantWrite)
	if err := td.WritePacketAt(writeBuf, Headroom); err != nil {
		t.Fatalf("WritePacketAt failed: %v", err)
	}
	if len(raw.written) == 0 || !bytes.Equal(raw.written[len(raw.written)-1], wantWrite) {
		t.Fatalf("WritePacketAt underlying Write got %x, want %x", raw.written, wantWrite)
	}

	if err := dev.WritePacket(nil); err != nil {
		t.Fatalf("WritePacket(nil) failed: %v", err)
	}
	if err := dev.WritePacket(wantWrite); err != nil {
		t.Fatalf("WritePacket failed: %v", err)
	}
	if !bytes.Equal(raw.written[len(raw.written)-1], wantWrite) {
		t.Fatalf("underlying Write got %x, want %x", raw.written[len(raw.written)-1], wantWrite)
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

func TestTunDeviceBatchReadWriteAndLinuxByteCountReturn(t *testing.T) {
	pkt1 := []byte{0x45, 0x00, 0x00, 0x18, 1, 1, 1, 1}
	pkt2 := []byte{0x45, 0x00, 0x00, 0x18, 2, 2, 2, 2}
	raw := &fakeWireGuardTUN{
		name:             "forti0",
		batchSize:        8,
		events:           make(chan tun.Event, 4),
		readBatches:      [][][]byte{{pkt1, pkt2}},
		readErrs:         []error{os.ErrClosed},
		writeReturnBytes: true, // Mirrors wireguard-go Linux NativeTun.Write returning byte count.
	}
	raw.mtu.Store(1400)

	dev, err := wrapTUN(raw, "forti0", 1400)
	if err != nil {
		t.Fatalf("wrapTUN failed: %v", err)
	}
	defer dev.Close()

	bdev, ok := dev.(BatchDevice)
	if !ok {
		t.Fatalf("expected wrapped TUN to implement BatchDevice")
	}
	if bdev.BatchSize() != 8 {
		t.Fatalf("BatchSize() = %d, want 8", bdev.BatchSize())
	}

	bufs := make([][]byte, 8)
	sizes := make([]int, 8)
	for i := range bufs {
		bufs[i] = make([]byte, Headroom+128)
	}

	// Trailing error with n > 0 must deliver the packets first, then surface the error on next call.
	n, err := bdev.ReadBatch(bufs, sizes, Headroom)
	if err != nil || n != 2 {
		t.Fatalf("ReadBatch = (%d, %v), want (2, nil)", n, err)
	}
	if !bytes.Equal(bufs[0][Headroom:Headroom+sizes[0]], pkt1) || !bytes.Equal(bufs[1][Headroom:Headroom+sizes[1]], pkt2) {
		t.Fatalf("unexpected batch packets")
	}
	n, err = bdev.ReadBatch(bufs, sizes, Headroom)
	if !errors.Is(err, os.ErrClosed) || n != 0 {
		t.Fatalf("second ReadBatch = (%d, %v), want (0, os.ErrClosed)", n, err)
	}

	// WriteBatch must succeed when Linux NativeTun.Write returns byte count instead of packet count.
	w1 := make([]byte, Headroom+len(pkt1))
	w2 := make([]byte, Headroom+len(pkt2))
	copy(w1[Headroom:], pkt1)
	copy(w2[Headroom:], pkt2)
	written, err := bdev.WriteBatch([][]byte{w1, w2}, Headroom)
	if err != nil || written != 2 {
		t.Fatalf("WriteBatch = (%d, %v), want (2, nil)", written, err)
	}
	if len(raw.written) != 2 || !bytes.Equal(raw.written[0], pkt1) || !bytes.Equal(raw.written[1], pkt2) {
		t.Fatalf("unexpected written batch: %x", raw.written)
	}
}

func TestTunDeviceSingleReadHandlesMultiSegmentBatch(t *testing.T) {
	pkt1 := []byte{0x45, 0x00, 0x00, 0x14, 0xaa}
	pkt2 := []byte{0x45, 0x00, 0x00, 0x14, 0xbb}
	pkt3 := []byte{0x45, 0x00, 0x00, 0x14, 0xcc}
	raw := &fakeWireGuardTUN{
		name:        "forti0",
		batchSize:   8,
		events:      make(chan tun.Event, 4),
		readBatches: [][][]byte{{pkt1, pkt2, pkt3}},
	}
	raw.mtu.Store(1400)

	dev, err := wrapTUN(raw, "forti0", 1400)
	if err != nil {
		t.Fatalf("wrapTUN failed: %v", err)
	}
	defer dev.Close()

	td := dev.(*tunDevice)
	buf := make([]byte, Headroom+64)
	for i, want := range [][]byte{pkt1, pkt2, pkt3} {
		n, err := td.ReadPacketAt(buf, Headroom)
		if err != nil {
			t.Fatalf("packet %d ReadPacketAt failed: %v", i, err)
		}
		if !bytes.Equal(buf[Headroom:Headroom+n], want) {
			t.Fatalf("packet %d got %x, want %x", i, buf[Headroom:Headroom+n], want)
		}
	}
}

func TestTunDeviceRejectsOversizedWriteAndTracksMTUUpdate(t *testing.T) {
	raw := newFakeWireGuardTUN("utun8", nil)
	dev, err := wrapTUN(raw, "utun8", 1400)
	if err != nil {
		t.Fatalf("wrapTUN failed: %v", err)
	}

	raw.mtu.Store(1360)
	raw.events <- tun.EventMTUUpdate
	deadline := time.Now().Add(time.Second)
	for dev.MTU() != 1360 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if dev.MTU() != 1360 {
		t.Fatalf("MTU() = %d, want 1360 after EventMTUUpdate", dev.MTU())
	}

	bdev := dev.(BatchDevice)
	if _, err := bdev.WriteBatch([][]byte{make([]byte, Headroom-1)}, Headroom); !errors.Is(err, ErrPacketRejected) {
		t.Fatalf("expected ErrPacketRejected on short batch buffer, got %v", err)
	}
	raw.writeErr = os.ErrClosed
	if err := dev.WritePacket([]byte{0x45, 0x00, 0x00, 0x14}); !errors.Is(err, os.ErrClosed) || errors.Is(err, ErrPacketRejected) {
		t.Fatalf("expected fatal os.ErrClosed without ErrPacketRejected, got %v", err)
	}

	if err := dev.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestTunDeviceErrTooManySegmentsDoesNotTerminateReads(t *testing.T) {
	pkt1 := []byte{0x45, 0x00, 0x00, 0x14, 0x11}
	pkt2 := []byte{0x45, 0x00, 0x00, 0x14, 0x22}
	pkt3 := []byte{0x45, 0x00, 0x00, 0x14, 0x33}
	raw := &fakeWireGuardTUN{
		name:        "forti0",
		batchSize:   8,
		events:      make(chan tun.Event, 4),
		readBatches: [][][]byte{{pkt1, pkt2}, {}, {pkt3}},
		readErrs:    []error{tun.ErrTooManySegments, tun.ErrTooManySegments, nil},
	}
	raw.mtu.Store(1400)

	dev, err := wrapTUN(raw, "forti0", 1400)
	if err != nil {
		t.Fatalf("wrapTUN failed: %v", err)
	}
	defer dev.Close()

	bdev := dev.(BatchDevice)
	bufs := make([][]byte, 8)
	sizes := make([]int, 8)
	for i := range bufs {
		bufs[i] = make([]byte, Headroom+128)
	}

	// First batch returns 2 valid segments alongside ErrTooManySegments -> must deliver both with nil error.
	n, err := bdev.ReadBatch(bufs, sizes, Headroom)
	if err != nil || n != 2 {
		t.Fatalf("first ReadBatch = (%d, %v), want (2, nil)", n, err)
	}
	if !bytes.Equal(bufs[0][Headroom:Headroom+sizes[0]], pkt1) || !bytes.Equal(bufs[1][Headroom:Headroom+sizes[1]], pkt2) {
		t.Fatalf("unexpected first batch packets")
	}

	// Second underlying read returns (0, ErrTooManySegments) and must transparently advance to third batch (pkt3).
	n, err = bdev.ReadBatch(bufs, sizes, Headroom)
	if err != nil || n != 1 || !bytes.Equal(bufs[0][Headroom:Headroom+sizes[0]], pkt3) {
		t.Fatalf("second ReadBatch = (%d, %v), want (1, nil) with pkt3", n, err)
	}

	// Single-packet ReadPacket must also tolerate ErrTooManySegments without poisoning readErr.
	rawSingle := &fakeWireGuardTUN{
		name:        "forti0",
		batchSize:   8,
		events:      make(chan tun.Event, 4),
		readBatches: [][][]byte{{pkt1}, {pkt2}},
		readErrs:    []error{tun.ErrTooManySegments, nil},
	}
	rawSingle.mtu.Store(1400)
	devSingle, err := wrapTUN(rawSingle, "forti0", 1400)
	if err != nil {
		t.Fatalf("wrapTUN single failed: %v", err)
	}
	defer devSingle.Close()
	buf := make([]byte, 64)
	for i, want := range [][]byte{pkt1, pkt2} {
		n, err := devSingle.ReadPacket(buf)
		if err != nil || !bytes.Equal(buf[:n], want) {
			t.Fatalf("single packet %d = (%x, %v), want (%x, nil)", i, buf[:n], err, want)
		}
	}
}

func TestTunDeviceWriteErrClassificationForJoinedErrors(t *testing.T) {
	rej := samplePacketRejectedErr()
	if rej == nil {
		t.Skip("no platform-specific packet-rejected errno on this OS")
	}
	raw := newFakeWireGuardTUN("forti0", nil)
	dev, err := wrapTUN(raw, "forti0", 1400)
	if err != nil {
		t.Fatalf("wrapTUN failed: %v", err)
	}
	defer dev.Close()
	bdev := dev.(BatchDevice)

	pkt := make([]byte, Headroom+20)
	pkt[Headroom] = 0x45

	// Purely recoverable joined errors -> classified as ErrPacketRejected.
	raw.writeErr = errors.Join(rej, rej)
	if _, err := bdev.WriteBatch([][]byte{pkt}, Headroom); !errors.Is(err, ErrPacketRejected) {
		t.Fatalf("pure recoverable joined error must wrap ErrPacketRejected, got %v", err)
	}

	// Joined error mixing recoverable + fatal device error -> MUST NOT be masked as ErrPacketRejected.
	raw.writeErr = errors.Join(rej, os.ErrClosed)
	if _, err := bdev.WriteBatch([][]byte{pkt}, Headroom); errors.Is(err, ErrPacketRejected) || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("mixed fatal joined error must not be masked as ErrPacketRejected, got %v", err)
	}
}
