// Package netdev provides a minimal cross-platform TUN device wrapper over
// golang.zx2c4.com/wireguard/tun with in-place headroom and batch I/O support.
package netdev

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"golang.zx2c4.com/wireguard/tun"

	"tunnelkit/fortivpn"
)

// Headroom is the leading byte offset reserved before the Layer-3 IPv4 packet
// so kernel TUN headers (4B BSD utun or 10B Linux virtio_net_hdr) and
// Fortinet/PPP framing (6B 0x5050 + 4B PPP = 10B) can be written in place.
const Headroom = 16

const (
	packetOffset  = Headroom
	maxPacketSize = 65535
	maxBatchSize  = 256
)

// ErrPacketRejected marks a write error that applies to an individual packet
// (such as EMSGSIZE or transient queue pressure) rather than fatal device failure.
var ErrPacketRejected = errors.New("netdev: packet rejected")

// Device represents an opened virtual network interface.
type Device interface {
	Name() string
	MTU() int
	ReadPacket(buf []byte) (int, error)
	WritePacket(pkt []byte) error
	Close() error
}

// BatchDevice extends Device with zero-extra-copy batched packet reads and writes
// aligned with wireguard/tun's BatchSize (e.g. Linux IFF_VNET_HDR GSO/GRO).
type BatchDevice interface {
	Device
	BatchSize() int
	ReadBatch(bufs [][]byte, sizes []int, offset int) (int, error)
	WriteBatch(bufs [][]byte, offset int) (int, error)
}

type tunDevice struct {
	dev       tun.Device
	name      string
	mtu       atomic.Int64
	batchSize int

	readMu      sync.Mutex
	singleBufs  [][]byte
	singleSizes []int
	singleNext  int
	singleCount int
	readErr     error

	writeMu   sync.Mutex
	writeBuf  []byte
	writeBufs [1][]byte

	closeOnce  sync.Once
	closeErr   error
	stopEvents chan struct{}
	eventsDone chan struct{}
}

func wrapTUN(dev tun.Device, fallbackName string, mtu int) (Device, error) {
	fail := func(msg string, cause error) (Device, error) {
		closeErr := dev.Close()
		var err error
		if cause != nil {
			err = fortivpn.WrapError(fortivpn.CodeDeviceFailed, msg, cause)
		} else {
			err = fortivpn.NewError(fortivpn.CodeDeviceFailed, msg)
		}
		return nil, errors.Join(err, closeErr)
	}

	name, err := dev.Name()
	if err != nil || name == "" {
		name = fallbackName
	}
	if name == "" {
		return fail("TUN device returned an empty interface name", err)
	}

	batch := dev.BatchSize()
	if batch < 1 || batch > maxBatchSize {
		return fail(fmt.Sprintf("invalid TUN batch size %d", batch), nil)
	}

	effectiveMTU := mtu
	if effectiveMTU <= 0 {
		if queried, qErr := dev.MTU(); qErr == nil && queried > 0 {
			effectiveMTU = queried
		} else {
			effectiveMTU = 1400
		}
	}

	td := &tunDevice{
		dev:        dev,
		name:       name,
		batchSize:  batch,
		writeBuf:   make([]byte, packetOffset+maxPacketSize),
		stopEvents: make(chan struct{}),
		eventsDone: make(chan struct{}),
	}
	td.mtu.Store(int64(effectiveMTU))

	// Drain tun.Events() so wireguard-go's event channel never blocks, and keep
	// MTU synchronized with kernel interface notifications.
	go func() {
		defer close(td.eventsDone)
		events := dev.Events()
		if events == nil {
			return
		}
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				if ev&tun.EventMTUUpdate != 0 {
					if queried, qErr := dev.MTU(); qErr == nil && queried > 0 {
						td.mtu.Store(int64(queried))
					}
				}
			case <-td.stopEvents:
				return
			}
		}
	}()
	return td, nil
}

func (t *tunDevice) Name() string   { return t.name }
func (t *tunDevice) MTU() int       { return int(t.mtu.Load()) }
func (t *tunDevice) BatchSize() int { return t.batchSize }

func (t *tunDevice) SetMTU(mtu int) {
	if mtu > 0 {
		t.mtu.Store(int64(mtu))
	}
}

func (t *tunDevice) ensureSingleReadBufsLocked() {
	if len(t.singleBufs) != 0 {
		return
	}
	t.singleBufs = make([][]byte, t.batchSize)
	t.singleSizes = make([]int, t.batchSize)
	for i := range t.singleBufs {
		t.singleBufs[i] = make([]byte, packetOffset+maxPacketSize)
	}
}

// ReadBatch reads up to min(len(bufs), BatchSize()) outbound IP packets directly
// into bufs[i][offset : offset+sizes[i]] without intermediate copying.
func (t *tunDevice) ReadBatch(bufs [][]byte, sizes []int, offset int) (int, error) {
	if offset < packetOffset || len(bufs) == 0 || len(sizes) < len(bufs) {
		return 0, io.ErrShortBuffer
	}
	t.readMu.Lock()
	defer t.readMu.Unlock()

	if t.singleNext < t.singleCount {
		copied := 0
		for t.singleNext < t.singleCount && copied < len(bufs) {
			n := t.singleSizes[t.singleNext]
			t.singleNext++
			if n < 0 || n > maxPacketSize {
				return copied, fmt.Errorf("invalid TUN packet length %d", n)
			}
			if offset+n > len(bufs[copied]) {
				return copied, io.ErrShortBuffer
			}
			copy(bufs[copied][offset:], t.singleBufs[t.singleNext-1][packetOffset:packetOffset+n])
			sizes[copied] = n
			copied++
		}
		return copied, nil
	}
	if t.readErr != nil {
		err := t.readErr
		t.readErr = nil
		return 0, err
	}

	limit := min(len(bufs), t.batchSize)
	for {
		n, err := t.dev.Read(bufs[:limit], sizes[:limit], offset)
		if n < 0 || n > limit {
			return 0, fmt.Errorf("invalid TUN read count %d", n)
		}
		for i := range n {
			if sizes[i] < 0 || sizes[i] > maxPacketSize || offset+sizes[i] > len(bufs[i]) {
				return 0, fmt.Errorf("invalid TUN packet length %d", sizes[i])
			}
		}
		// wireguard-go contract: ErrTooManySegments means bufs[:n] holds valid GSO
		// segments and excess segments were truncated; never terminate the reader loop.
		if errors.Is(err, tun.ErrTooManySegments) {
			if n > 0 {
				return n, nil
			}
			continue
		}
		if n > 0 && err != nil {
			// Defer trailing read errors so already-decoded packets are delivered first.
			t.readErr = err
			return n, nil
		}
		return n, err
	}
}

// ReadPacketAt reads one outbound IP packet directly into buf[offset : offset+n]
// when BatchSize()==1, or safely drains a BatchSize()-sized GSO buffer on Linux.
func (t *tunDevice) ReadPacketAt(buf []byte, offset int) (int, error) {
	if len(buf) <= offset {
		return 0, nil
	}
	if offset < packetOffset || t.batchSize > 1 {
		return t.ReadPacket(buf[offset:])
	}
	t.readMu.Lock()
	defer t.readMu.Unlock()

	if t.readErr != nil {
		err := t.readErr
		t.readErr = nil
		return 0, err
	}
	readBufs := [1][]byte{buf}
	var readSizes [1]int
	for {
		n, err := t.dev.Read(readBufs[:], readSizes[:], offset)
		if n < 0 || n > 1 {
			return 0, fmt.Errorf("invalid TUN read count %d", n)
		}
		if errors.Is(err, tun.ErrTooManySegments) {
			if n == 0 || readSizes[0] <= 0 {
				continue
			}
			err = nil
		}
		if n > 0 && err != nil {
			t.readErr = err
			err = nil
		}
		if err != nil {
			return 0, err
		}
		if n == 0 || readSizes[0] <= 0 {
			return 0, nil
		}
		if readSizes[0] > maxPacketSize || offset+readSizes[0] > len(buf) {
			return 0, fmt.Errorf("invalid TUN packet length %d", readSizes[0])
		}
		return readSizes[0], nil
	}
}

func (t *tunDevice) ReadPacket(buf []byte) (int, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	t.ensureSingleReadBufsLocked()
	for t.singleNext == t.singleCount {
		if t.readErr != nil {
			err := t.readErr
			t.readErr = nil
			return 0, err
		}
		t.singleNext = 0
		count, err := t.dev.Read(t.singleBufs, t.singleSizes, packetOffset)
		if count < 0 || count > len(t.singleBufs) {
			return 0, fmt.Errorf("invalid TUN read count %d", count)
		}
		if errors.Is(err, tun.ErrTooManySegments) {
			if count == 0 {
				continue
			}
			t.singleCount = count
			break
		}
		t.singleCount = count
		if count > 0 && err != nil {
			t.readErr = err
		} else if err != nil {
			return 0, err
		} else if count == 0 {
			return 0, nil
		}
	}

	idx := t.singleNext
	t.singleNext++
	size := t.singleSizes[idx]
	if size < 0 || size > maxPacketSize {
		return 0, fmt.Errorf("invalid TUN packet length %d", size)
	}
	if size > len(buf) {
		return 0, io.ErrShortBuffer
	}
	return copy(buf, t.singleBufs[idx][packetOffset:packetOffset+size]), nil
}

// WriteBatch writes a batch of inbound IP packets located at bufs[i][offset:]
// directly to the kernel TUN device using bufs[i][:offset] as kernel headroom.
func (t *tunDevice) WriteBatch(bufs [][]byte, offset int) (int, error) {
	if len(bufs) == 0 {
		return 0, nil
	}
	if offset < packetOffset {
		return 0, errors.Join(ErrPacketRejected, io.ErrShortBuffer)
	}
	for _, buf := range bufs {
		if len(buf) <= offset || len(buf)-offset > maxPacketSize {
			return 0, errors.Join(ErrPacketRejected, io.ErrShortBuffer)
		}
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	// Note: wireguard/tun's Linux NativeTun.Write returns written bytes (after GRO)
	// whereas Darwin/Windows return packet count. Treat err == nil as full batch delivery.
	_, err := t.dev.Write(bufs, offset)
	if err != nil {
		return 0, t.classifyWriteErr(err)
	}
	return len(bufs), nil
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
	if len(buf)-offset > maxPacketSize {
		return errors.Join(ErrPacketRejected, io.ErrShortBuffer)
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	t.writeBufs[0] = buf
	_, err := t.dev.Write(t.writeBufs[:], offset)
	return t.classifyWriteErr(err)
}

func (t *tunDevice) WritePacket(pkt []byte) error {
	if len(pkt) == 0 {
		return nil
	}
	if len(pkt) > maxPacketSize {
		return errors.Join(ErrPacketRejected, io.ErrShortBuffer)
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	end := packetOffset + len(pkt)
	copy(t.writeBuf[packetOffset:end], pkt)
	t.writeBufs[0] = t.writeBuf[:end]
	_, err := t.dev.Write(t.writeBufs[:], packetOffset)
	return t.classifyWriteErr(err)
}

func (t *tunDevice) classifyWriteErr(err error) error {
	if err == nil {
		return nil
	}
	if isRecoverableWriteErr(err) {
		if queried, qErr := t.dev.MTU(); qErr == nil && queried > 0 {
			t.mtu.Store(int64(queried))
		}
		return errors.Join(ErrPacketRejected, err)
	}
	return err
}

// isRecoverableWriteErr recursively inspects wrapped and joined errors (such as
// Linux NativeTun.Write's errors.Join) and returns true only when every non-nil
// leaf error is a recoverable per-packet error, preventing fatal device failures
// mixed into a batch from being masked as ErrPacketRejected.
func isRecoverableWriteErr(err error) bool {
	if err == nil {
		return false
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		leaves := multi.Unwrap()
		hasLeaf := false
		for _, sub := range leaves {
			if sub == nil {
				continue
			}
			hasLeaf = true
			if !isRecoverableWriteErr(sub) {
				return false
			}
		}
		return hasLeaf
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		if inner := single.Unwrap(); inner != nil {
			return isRecoverableWriteErr(inner)
		}
	}
	return isPacketRejectedErr(err)
}

func (t *tunDevice) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = t.dev.Close()
		close(t.stopEvents)
		<-t.eventsDone
	})
	return t.closeErr
}
