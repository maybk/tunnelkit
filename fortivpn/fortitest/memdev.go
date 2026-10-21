package fortitest

import (
	"context"
	"io"
	"sync"
)

// MemDevice is an in-memory TUN device implementing engine.Device for rootless tests.
type MemDevice struct {
	name      string
	mtu       int
	inbound   chan []byte // packets written by the engine (from gateway to host)
	outbound  chan []byte // packets read by the engine (from host to gateway)
	closed    chan struct{}
	closeOnce sync.Once
}

// NewMemDevice creates an in-memory TUN device with the given name and MTU.
func NewMemDevice(name string, mtu int) *MemDevice {
	if name == "" {
		name = "utun99"
	}
	if mtu <= 0 {
		mtu = 1400
	}
	return &MemDevice{
		name:     name,
		mtu:      mtu,
		inbound:  make(chan []byte, 256),
		outbound: make(chan []byte, 256),
		closed:   make(chan struct{}),
	}
}

func (m *MemDevice) Name() string { return m.name }
func (m *MemDevice) MTU() int     { return m.mtu }

// ReadPacket is called by the engine to read outbound IP packets originating from the host.
func (m *MemDevice) ReadPacket(buf []byte) (int, error) {
	select {
	case <-m.closed:
		return 0, io.EOF
	case pkt := <-m.outbound:
		n := copy(buf, pkt)
		return n, nil
	}
}

// WritePacket is called by the engine to deliver inbound IP packets arriving from the gateway.
func (m *MemDevice) WritePacket(pkt []byte) error {
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	select {
	case <-m.closed:
		return io.ErrClosedPipe
	case m.inbound <- cp:
		return nil
	}
}

// Close closes the in-memory device.
func (m *MemDevice) Close() error {
	m.closeOnce.Do(func() {
		close(m.closed)
	})
	return nil
}

// SendFromHost injects an outbound IPv4 packet from the host into the tunnel.
func (m *MemDevice) SendFromHost(pkt []byte) error {
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	select {
	case <-m.closed:
		return io.ErrClosedPipe
	case m.outbound <- cp:
		return nil
	}
}

// ReceiveOnHost waits for the next inbound IPv4 packet delivered to the host.
func (m *MemDevice) ReceiveOnHost(ctx context.Context) ([]byte, error) {
	select {
	case <-m.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	case pkt := <-m.inbound:
		return pkt, nil
	}
}
