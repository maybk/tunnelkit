package engine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/fortitest"
	"tunnelkit/fortivpn/internal/netdev"
	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

func makeTestIPv4(size int, tag byte) []byte {
	pkt := bytes.Repeat([]byte{tag}, size)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(size))
	copy(pkt[12:16], []byte{10, 0, 0, 2})
	copy(pkt[16:20], []byte{10, 0, 0, 1})
	return pkt
}

func TestDeviceReaderPreservesIPv4Packets(t *testing.T) {
	for _, size := range []int{1400, 3000, 9000, 16384, sslvpn.MaxFramePayload - ppp.HeaderLen} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dev := fortitest.NewMemDevice("test", size)
			s := &session{}
			l := &link{dataCh: make(chan *[]byte, 1), doneCh: make(chan struct{})}
			s.activeLink.Store(l)
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.deviceReaderLoop(dev)
			}()
			t.Cleanup(func() { _ = dev.Close(); <-done })

			packet := makeTestIPv4(size, 0xa5)
			if err := dev.SendFromHost(packet); err != nil {
				t.Fatal(err)
			}
			select {
			case buf := <-l.dataCh:
				defer putDataFrameBuf(buf)
				frame, rest, err := sslvpn.ParseFrame((*buf)[outboundFrameOffset:])
				if err != nil || len(rest) != 0 {
					t.Fatalf("invalid tunnel frame: %v", err)
				}
				got, ok := ppp.ExtractIPv4(frame)
				if !ok || !bytes.Equal(got, packet) {
					t.Fatal("outbound packet changed or was truncated")
				}
			case <-time.After(time.Second):
				t.Fatal("outbound packet did not reach writer")
			}
		})
	}
}

type fakeBatchDevice struct {
	mu          sync.Mutex
	batchSize   int
	readBatches [][][]byte
	readIdx     int
	written     [][][]byte
	writeErr    error
	closed      chan struct{}
}

func newFakeBatchDevice(batchSize int, batches ...[][]byte) *fakeBatchDevice {
	return &fakeBatchDevice{
		batchSize:   batchSize,
		readBatches: batches,
		closed:      make(chan struct{}),
	}
}

func (f *fakeBatchDevice) Name() string                   { return "forti0" }
func (f *fakeBatchDevice) MTU() int                       { return 1400 }
func (f *fakeBatchDevice) BatchSize() int                 { return f.batchSize }
func (f *fakeBatchDevice) ReadPacket([]byte) (int, error) { return 0, io.EOF }
func (f *fakeBatchDevice) WritePacket(pkt []byte) error   { return f.writeErr }
func (f *fakeBatchDevice) Close() error                   { close(f.closed); return nil }
func (f *fakeBatchDevice) ReadBatch(bufs [][]byte, sizes []int, offset int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readIdx >= len(f.readBatches) {
		return 0, io.EOF
	}
	batch := f.readBatches[f.readIdx]
	f.readIdx++
	for i, pkt := range batch {
		sizes[i] = copy(bufs[i][offset:], pkt)
	}
	return len(batch), nil
}

func (f *fakeBatchDevice) WriteBatch(bufs [][]byte, offset int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	cp := make([][]byte, len(bufs))
	for i, b := range bufs {
		cp[i] = append([]byte(nil), b[offset:]...)
	}
	f.written = append(f.written, cp)
	return len(bufs), nil
}

func TestDeviceReaderBatchLoopReadsMultipleSegments(t *testing.T) {
	p1 := makeTestIPv4(120, 0x11)
	p2 := makeTestIPv4(240, 0x22)
	p3 := makeTestIPv4(360, 0x33)
	dev := newFakeBatchDevice(8, [][]byte{p1, p2, p3})

	s := &session{}
	l := &link{dataCh: make(chan *[]byte, 8), doneCh: make(chan struct{})}
	s.activeLink.Store(l)
	s.deviceReaderLoop(dev)

	if len(l.dataCh) != 3 {
		t.Fatalf("queued packets = %d, want 3", len(l.dataCh))
	}
	for i, want := range [][]byte{p1, p2, p3} {
		buf := <-l.dataCh
		frame, rest, err := sslvpn.ParseFrame((*buf)[outboundFrameOffset:])
		putDataFrameBuf(buf)
		if err != nil || len(rest) != 0 {
			t.Fatalf("packet %d invalid frame: %v", i, err)
		}
		got, ok := ppp.ExtractIPv4(frame)
		if !ok || !bytes.Equal(got, want) {
			t.Fatalf("packet %d mismatch", i)
		}
	}
}

type recordWriteConn struct {
	net.Conn
	mu     sync.Mutex
	writes [][]byte
}

func (c *recordWriteConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes = append(c.writes, append([]byte(nil), p...))
	c.mu.Unlock()
	return len(p), nil
}
func (c *recordWriteConn) SetWriteDeadline(time.Time) error { return nil }
func (c *recordWriteConn) SetDeadline(time.Time) error      { return nil }
func (c *recordWriteConn) Close() error                     { return nil }

func TestWriterCoalescesQueuedFramesAndRespectsControlPriority(t *testing.T) {
	rec := &recordWriteConn{}
	s := &session{}
	l := &link{
		sess:   s,
		tconn:  sslvpn.NewTunnelConn(rec),
		ctrlCh: make(chan []byte, 4),
		dataCh: make(chan *[]byte, 16),
		doneCh: make(chan struct{}),
	}

	// Queue 4 small packets: all 4 should coalesce into a single TLS write.
	wants := [][]byte{
		makeTestIPv4(200, 0x01),
		makeTestIPv4(300, 0x02),
		makeTestIPv4(400, 0x03),
		makeTestIPv4(500, 0x04),
	}
	for _, p := range wants {
		l.enqueueData(p)
	}
	first := <-l.dataCh
	overflow, err := l.writeCoalescedData(first, make([]byte, maxOutboundCoalesceBytes))
	if err != nil || overflow != nil {
		t.Fatalf("writeCoalescedData = (overflow=%v, err=%v), want (nil, nil)", overflow != nil, err)
	}

	if len(rec.writes) != 1 {
		t.Fatalf("TLS write count = %d, want 1 coalesced write", len(rec.writes))
	}
	rest := rec.writes[0]
	for i, want := range wants {
		frame, next, err := sslvpn.ParseFrame(rest)
		if err != nil {
			t.Fatalf("frame %d parse error: %v", i, err)
		}
		got, ok := ppp.ExtractIPv4(frame)
		if !ok || !bytes.Equal(got, want) {
			t.Fatalf("frame %d payload mismatch", i)
		}
		rest = next
	}
	if len(rest) != 0 {
		t.Fatalf("unexpected trailing bytes: %d", len(rest))
	}
	if s.txPackets.Load() != 4 || s.txBytes.Load() != 200+300+400+500 {
		t.Fatalf("stats = (%d pkts, %d bytes), want (4, 1400)", s.txPackets.Load(), s.txBytes.Load())
	}

	// Overflow packet should seed the next coalesced batch rather than causing an isolated write.
	rec.writes = nil
	smallBuf := make([]byte, 700)
	for i := range 4 {
		l.enqueueData(makeTestIPv4(300, byte(0x10+i)))
	}
	pending := <-l.dataCh
	for pending != nil || len(l.dataCh) > 0 {
		if pending == nil {
			pending = <-l.dataCh
		}
		pending, err = l.writeCoalescedData(pending, smallBuf)
		if err != nil {
			t.Fatalf("coalesced overflow batch failed: %v", err)
		}
	}
	if len(rec.writes) != 2 {
		t.Fatalf("overflow coalesced write count = %d, want 2 full batches of 2 packets", len(rec.writes))
	}
}

func TestReaderBatchStepBatchesInboundAndHandlesMixedPPPHeadersAndFatalErrors(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	p1 := makeTestIPv4(100, 0x41)
	p2 := makeTestIPv4(120, 0x42)
	p3 := makeTestIPv4(140, 0x43)

	// Build two 4-byte PPP frames and one compressed 2-byte PPP frame back-to-back.
	var wire bytes.Buffer
	f1 := make([]byte, ppp.HeaderLen+len(p1))
	ppp.PutFrameHeader(f1[:ppp.HeaderLen], ppp.ProtocolIP)
	copy(f1[ppp.HeaderLen:], p1)
	wire.Write(sslvpn.EncodeFrame(f1))

	f2 := make([]byte, ppp.HeaderLen+len(p2))
	ppp.PutFrameHeader(f2[:ppp.HeaderLen], ppp.ProtocolIP)
	copy(f2[ppp.HeaderLen:], p2)
	wire.Write(sslvpn.EncodeFrame(f2))

	f3 := append([]byte{0x00, 0x21}, p3...)
	wire.Write(sslvpn.EncodeFrame(f3))

	go func() {
		_, _ = server.Write(wire.Bytes())
		_ = server.Close()
	}()

	dev := newFakeBatchDevice(8)
	s := &session{dev: dev}
	l := &link{
		sess:   s,
		tconn:  sslvpn.NewTunnelConn(client),
		doneCh: make(chan struct{}),
	}

	if err := l.readerLoop(); err != nil && !fortivpn.IsCode(err, fortivpn.CodeConnectionLost) {
		t.Fatalf("unexpected readerLoop error: %v", err)
	}
	if s.rxPackets.Load() != 3 {
		t.Fatalf("rxPackets = %d, want 3", s.rxPackets.Load())
	}
	if len(dev.written) != 2 {
		t.Fatalf("WriteBatch calls = %d, want 2 (2x 4B-PPP batch + 1x 2B-PPP batch)", len(dev.written))
	}
	if len(dev.written[0]) != 2 || !bytes.Equal(dev.written[0][0], p1) || !bytes.Equal(dev.written[0][1], p2) {
		t.Fatalf("first batch mismatch")
	}
	if len(dev.written[1]) != 1 || !bytes.Equal(dev.written[1][0], p3) {
		t.Fatalf("second batch mismatch")
	}

	// Verify rejected packet errors are ignored while fatal device errors propagate CodeDeviceFailed.
	if err := l.classifyInboundWriteErr(errors.Join(netdev.ErrPacketRejected, errors.New("emsgsize"))); err != nil {
		t.Fatalf("expected ErrPacketRejected to be ignored, got %v", err)
	}
	if err := l.classifyInboundWriteErr(os.ErrClosed); !fortivpn.IsCode(err, fortivpn.CodeDeviceFailed) {
		t.Fatalf("expected fatal write error to return CodeDeviceFailed, got %v", err)
	}
}

func TestDetectWakeJump(t *testing.T) {
	if !isWakeJump(2*time.Second, 30*time.Second, 2*time.Second, wakeJumpThreshold) {
		t.Fatal("expected S3 wall/monotonic skew to detect wake jump")
	}
	if !isWakeJump(25*time.Second, 25*time.Second, 2*time.Second, wakeJumpThreshold) {
		t.Fatal("expected Windows S0 suspended-timer monotonic jump to detect wake jump")
	}
	if isWakeJump(2*time.Second, 2*time.Second, 2*time.Second, wakeJumpThreshold) {
		t.Fatal("normal tick must not trigger wake jump")
	}
	if isWakeJump(2*time.Second, -60*time.Second, 2*time.Second, wakeJumpThreshold) {
		t.Fatal("backward NTP wall jump must not trigger wake jump")
	}
	if detectWakeJump(time.Time{}, time.Now(), 2*time.Second, wakeJumpThreshold) {
		t.Fatal("zero prev time must not trigger wake jump")
	}
}
