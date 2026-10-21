package engine

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/ppp"
)

const (
	ctrlQueueSize = 64
	dataQueueSize = 256
)

var packetBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 2048)
		return &b
	},
}

// enqueueControl pushes a PPP control frame (LCP/IPCP/Echo) onto ctrlCh without dropping.
func (l *link) enqueueControl(frame []byte) {
	cp := make([]byte, len(frame))
	copy(cp, frame)
	select {
	case l.ctrlCh <- cp:
	case <-l.doneCh:
	}
}

// shouldDropOutboundIPv4 filters non-routable local IPv4 noise (multicast,
// broadcast, link-local, loopback, and unspecified destinations).
func shouldDropOutboundIPv4(pkt []byte) bool {
	if len(pkt) < 20 {
		return true
	}
	dst := netip.AddrFrom4([4]byte{pkt[16], pkt[17], pkt[18], pkt[19]})
	if !dst.IsValid() || dst.IsUnspecified() || dst.IsLoopback() || dst.IsMulticast() || dst.IsLinkLocalUnicast() {
		return true
	}
	return dst == netip.AddrFrom4([4]byte{255, 255, 255, 255})
}

// enqueueData pushes an encapsulated IPv4 PPP frame onto dataCh, dropping
// non-IPv4/noise packets or on congestion.
func (l *link) enqueueData(ipPkt []byte) {
	trimmed, ok := ppp.TrimToIPv4(ipPkt)
	if !ok || shouldDropOutboundIPv4(trimmed) {
		return
	}
	frame := ppp.EncapsulateIP(trimmed)
	select {
	case l.dataCh <- frame:
	case <-l.doneCh:
	default:
		// Drop packet on congestion; never block the TUN reader.
	}
}

func (l *link) writerLoop() error {
	for {
		select {
		case <-l.doneCh:
			return nil
		case frame := <-l.ctrlCh:
			if err := l.writeFrame(frame, false); err != nil {
				return err
			}
		default:
			select {
			case <-l.doneCh:
				return nil
			case frame := <-l.ctrlCh:
				if err := l.writeFrame(frame, false); err != nil {
					return err
				}
			case frame := <-l.dataCh:
				if err := l.writeFrame(frame, true); err != nil {
					return err
				}
			}
		}
	}
}

func (l *link) writeFrame(frame []byte, isData bool) error {
	_ = l.tconn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	err := l.tconn.WriteFrame(frame)
	_ = l.tconn.SetWriteDeadline(time.Time{})
	if err != nil {
		if l.isClosing() {
			return nil
		}
		return fortivpn.WrapError(fortivpn.CodeConnectionLost, "tunnel TLS write failed", err)
	}
	if isData && len(frame) > 4 {
		l.sess.txBytes.Add(uint64(len(frame) - 4))
		l.sess.txPackets.Add(1)
	}
	return nil
}

func (l *link) readerLoop() error {
	bufPtr := packetBufPool.Get().(*[]byte)
	defer packetBufPool.Put(bufPtr)
	buf := *bufPtr

	for {
		select {
		case <-l.doneCh:
			return nil
		default:
		}

		frame, err := l.tconn.ReadFrameInto(buf)
		if err != nil {
			if l.isClosing() || errors.Is(err, context.Canceled) {
				return nil
			}
			var fe *fortivpn.Error
			if errors.As(err, &fe) {
				return fe
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return fortivpn.WrapError(fortivpn.CodeConnectionLost, "gateway closed SSL-VPN tunnel connection", err)
			}
			return fortivpn.WrapError(fortivpn.CodeConnectionLost, "tunnel TLS read failed", err)
		}

		if len(frame) == 0 {
			continue
		}

		if ipPkt, ok := ppp.ExtractIPv4(frame); ok {
			l.sess.rxBytes.Add(uint64(len(ipPkt)))
			l.sess.rxPackets.Add(1)
			if dev := l.sess.getDevice(); dev != nil {
				cp := make([]byte, len(ipPkt))
				copy(cp, ipPkt)
				_ = dev.WritePacket(cp)
			}
			continue
		}

		l.pppSess.Input(frame)
	}
}

// runConnectedLoops runs the LCP Echo keepalive loop and Stats emitter loop
// alongside the active reader/writer goroutines until ctx is canceled or the
// link fails.
func (l *link) runConnectedLoops(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)

	// Monitor reader/writer termination
	g.Go(func() error {
		select {
		case <-gCtx.Done():
			return nil
		case err := <-l.ioErrCh:
			return err
		}
	})

	// LCP Echo keepalive loop
	g.Go(func() error {
		ticker := time.NewTicker(l.sess.opts.echoInterval)
		defer ticker.Stop()

		failures := 0
		for {
			select {
			case <-gCtx.Done():
				return nil
			case <-l.doneCh:
				return nil
			case <-ticker.C:
				probeCtx, cancel := context.WithTimeout(gCtx, l.sess.opts.echoTimeout)
				rtt, err := l.pppSess.SendEcho(probeCtx)
				cancel()
				if err != nil {
					if gCtx.Err() != nil || l.isClosing() {
						return nil
					}
					failures++
					if failures >= l.sess.opts.maxEchoFailures {
						return fortivpn.NewError(fortivpn.CodePeerDead, "LCP echo keepalive timed out")
					}
					continue
				}
				failures = 0
				ms := rtt.Milliseconds()
				if ms < 1 && rtt > 0 {
					ms = 1
				}
				atomic.StoreInt64(&l.sess.rttMillis, ms)
			}
		}
	})

	// Periodic Stats event emitter
	g.Go(func() error {
		ticker := time.NewTicker(l.sess.opts.statsInterval)
		defer ticker.Stop()

		for {
			select {
			case <-gCtx.Done():
				return nil
			case <-l.doneCh:
				return nil
			case <-ticker.C:
				stats := l.sess.snapshotStats()
				l.sess.emit(fortivpn.Event{
					Phase: fortivpn.PhaseConnected,
					Stats: &stats,
				})
			}
		}
	})

	return g.Wait()
}
