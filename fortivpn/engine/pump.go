package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"time"

	"golang.org/x/sync/errgroup"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netdev"
	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

const (
	ctrlQueueSize = 64
	dataQueueSize = 256
)

func (s *session) ensureDevice(mru int) error {
	s.mu.Lock()
	if s.dev != nil {
		dev := s.dev
		s.mu.Unlock()
		if setter, ok := dev.(interface{ SetMTU(int) }); ok && mru > 0 {
			setter.SetMTU(mru)
		}
		return nil
	}
	var (
		dev Device
		err error
	)
	if s.opts.device != nil {
		dev = s.opts.device
	} else {
		dev, err = netdev.Open(mru)
		if err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.dev = dev
	s.mu.Unlock()

	s.log.Info(fmt.Sprintf("Interface %s is UP.", dev.Name()))

	s.devWg.Go(func() {
		s.deviceReaderLoop(dev)
	})
	return nil
}

func (s *session) deviceReaderLoop(dev Device) {
	if bdev, ok := dev.(batchDevice); ok && bdev.BatchSize() > 1 {
		s.deviceReaderBatchLoop(bdev)
		return
	}

	fastDev, hasFast := dev.(headroomDevice)
	var fallbackScratch []byte
	if !hasFast {
		fallbackScratch = make([]byte, 65535)
	}

	for {
		if hasFast {
			// Reserve the full IPv4 capacity, including across MTU changes while Read blocks.
			bufPtr := getDataFrameBuf(maxIPv4PacketSize)
			buf := (*bufPtr)[:cap(*bufPtr)]
			n, err := fastDev.ReadPacketAt(buf, outboundIPOffset)
			if err != nil {
				putDataFrameBuf(bufPtr)
				return
			}
			if n <= 0 {
				putDataFrameBuf(bufPtr)
				continue
			}
			if lnk := s.activeLink.Load(); lnk != nil {
				lnk.enqueueDataHeadroom(bufPtr, n)
			} else {
				putDataFrameBuf(bufPtr)
			}
			continue
		}

		n, err := dev.ReadPacket(fallbackScratch)
		if err != nil {
			return
		}
		if n <= 0 {
			continue
		}
		if lnk := s.activeLink.Load(); lnk != nil {
			lnk.enqueueData(fallbackScratch[:n])
		}
	}
}

func (s *session) getDevice() Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dev
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

// enqueueDataHeadroom encapsulates an IPv4 packet already located at (*bufPtr)[outboundIPOffset:outboundIPOffset+n]
// in-place with PPP and Fortinet headers without copying payload bytes.
func (l *link) enqueueDataHeadroom(bufPtr *[]byte, n int) {
	buf := (*bufPtr)[:outboundIPOffset+n]
	trimmed, ok := ppp.TrimToIPv4(buf[outboundIPOffset:])
	if !ok || shouldDropOutboundIPv4(trimmed) {
		putDataFrameBuf(bufPtr)
		return
	}
	ipLen := len(trimmed)
	pppLen := ppp.HeaderLen + ipLen
	if pppLen > sslvpn.MaxFramePayload {
		putDataFrameBuf(bufPtr)
		return
	}
	ppp.PutFrameHeader(buf[outboundPPPOffset:outboundIPOffset], ppp.ProtocolIP)
	sslvpn.PutFrameHeader(buf[outboundFrameOffset:outboundPPPOffset], pppLen)
	*bufPtr = buf[:outboundIPOffset+ipLen]

	select {
	case l.dataCh <- bufPtr:
	case <-l.doneCh:
		putDataFrameBuf(bufPtr)
	default:
		// Drop packet on congestion; never block the TUN reader.
		putDataFrameBuf(bufPtr)
	}
}

// enqueueData encapsulates a raw IPv4 packet slice into a pooled headroom buffer.
func (l *link) enqueueData(ipPkt []byte) {
	trimmed, ok := ppp.TrimToIPv4(ipPkt)
	if !ok || shouldDropOutboundIPv4(trimmed) {
		return
	}
	ipLen := len(trimmed)
	need := outboundIPOffset + ipLen
	bufPtr := getDataFrameBuf(ipLen)
	buf := (*bufPtr)[:need]
	copy(buf[outboundIPOffset:], trimmed)
	*bufPtr = buf
	l.enqueueDataHeadroom(bufPtr, ipLen)
}

func (l *link) writerLoop() error {
	coalesceBuf := make([]byte, maxOutboundCoalesceBytes)
	var pendingData *[]byte
	defer func() {
		if pendingData != nil {
			putDataFrameBuf(pendingData)
		}
	}()

	var deadlineEnd time.Time
	armDeadline := func() {
		now := time.Now()
		if now.Add(5 * time.Second).After(deadlineEnd) {
			deadlineEnd = now.Add(15 * time.Second)
			_ = l.tconn.SetWriteDeadline(deadlineEnd)
		}
	}
	clearDeadline := func() {
		if !deadlineEnd.IsZero() {
			deadlineEnd = time.Time{}
			_ = l.tconn.SetWriteDeadline(time.Time{})
		}
	}
	defer clearDeadline()

	for {
		select {
		case <-l.doneCh:
			return nil
		case frame := <-l.ctrlCh:
			armDeadline()
			if err := l.writeControlFrame(frame); err != nil {
				return err
			}
		default:
			if pendingData != nil {
				armDeadline()
				next := pendingData
				pendingData = nil
				overflow, err := l.writeCoalescedData(next, coalesceBuf)
				if err != nil {
					return err
				}
				pendingData = overflow
				continue
			}
			if len(l.ctrlCh) == 0 && len(l.dataCh) == 0 {
				clearDeadline()
			}
			select {
			case <-l.doneCh:
				return nil
			case frame := <-l.ctrlCh:
				armDeadline()
				if err := l.writeControlFrame(frame); err != nil {
					return err
				}
			case bufPtr := <-l.dataCh:
				armDeadline()
				overflow, err := l.writeCoalescedData(bufPtr, coalesceBuf)
				if err != nil {
					return err
				}
				pendingData = overflow
			}
		}
	}
}

func (l *link) writeControlFrame(pppFrame []byte) error {
	if err := l.tconn.WriteFrame(pppFrame); err != nil {
		if l.isClosing() {
			return nil
		}
		return fortivpn.WrapError(fortivpn.CodeConnectionLost, "tunnel TLS write failed", err)
	}
	return nil
}

func (l *link) readerLoop() error {
	bufPtr := packetBufPool.Get().(*[]byte)
	defer packetBufPool.Put(bufPtr)
	buf := (*bufPtr)[:cap(*bufPtr)]
	readScratch := buf[netdev.Headroom:]

	var batchWriter *inboundBatchWriter

	for {
		select {
		case <-l.doneCh:
			return nil
		default:
		}

		dev := l.sess.getDevice()
		bdev, useBatch := dev.(batchDevice)
		if useBatch && bdev.BatchSize() > 1 {
			if batchWriter == nil {
				batchWriter = newInboundBatchWriter(bdev.BatchSize())
			}
			done, err := l.readBatchStep(dev, bdev, batchWriter)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}

		frame, done, err := l.readTunnelFrame(readScratch)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if len(frame) == 0 {
			continue
		}

		if ipPkt, ok := ppp.ExtractIPv4(frame); ok {
			l.sess.rxBytes.Add(uint64(len(ipPkt)))
			l.sess.rxPackets.Add(1)
			// Re-fetch device after read so the first packet arriving right as
			// ensureDevice finishes opening the TUN interface is never dropped.
			if curDev := l.sess.getDevice(); curDev != nil {
				if err := l.writeSingleInbound(curDev, buf, readScratch, frame, ipPkt); err != nil {
					return err
				}
			}
			continue
		}

		l.pppSess.Input(frame)
	}
}

func (l *link) readBatchStep(dev Device, bdev batchDevice, w *inboundBatchWriter) (bool, error) {
	for {
		scratch := w.readScratch()
		frame, done, err := l.readTunnelFrame(scratch)
		if err != nil {
			_ = w.flush(bdev)
			return false, err
		}
		if done {
			return true, l.classifyInboundWriteErr(w.flush(bdev))
		}
		if len(frame) == 0 {
			if !l.canContinueInboundBatch(w) {
				return false, l.classifyInboundWriteErr(w.flush(bdev))
			}
			continue
		}

		if ipPkt, ok := ppp.ExtractIPv4(frame); ok {
			l.sess.rxBytes.Add(uint64(len(ipPkt)))
			l.sess.rxPackets.Add(1)
			needFlush, off := w.add(frame, ipPkt)
			if needFlush {
				if err := l.classifyInboundWriteErr(w.flush(bdev)); err != nil {
					return false, err
				}
				if off < 0 {
					if err := l.classifyInboundWriteErr(dev.WritePacket(ipPkt)); err != nil {
						return false, err
					}
					return false, nil
				}
				// Re-stage packet into slot 0 at its exact offset.
				slot := w.slots[0]
				copy(slot[off:], ipPkt)
				w.pendingOff = off
				w.pending = append(w.pending, slot[:off+len(ipPkt)])
				w.count = 1
			}
			if !l.canContinueInboundBatch(w) {
				return false, l.classifyInboundWriteErr(w.flush(bdev))
			}
			continue
		}

		if err := l.classifyInboundWriteErr(w.flush(bdev)); err != nil {
			return false, err
		}
		l.pppSess.Input(frame)
		return false, nil
	}
}

func (l *link) readTunnelFrame(scratch []byte) ([]byte, bool, error) {
	frame, err := l.tconn.ReadFrameInto(scratch)
	if err == nil {
		return frame, false, nil
	}
	if l.isClosing() || errors.Is(err, context.Canceled) {
		return nil, true, nil
	}
	if fe, ok := errors.AsType[*fortivpn.Error](err); ok {
		return nil, false, fe
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, false, fortivpn.WrapError(fortivpn.CodeConnectionLost, "gateway closed SSL-VPN tunnel connection", err)
	}
	return nil, false, fortivpn.WrapError(fortivpn.CodeConnectionLost, "tunnel TLS read failed", err)
}

// runConnectedLoops runs the LCP Echo keepalive loop (with sleep/wake detection
// and post-wake DNS reconciliation) and Stats emitter loop alongside reader/writer.
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

	// LCP Echo keepalive & sleep/wake detection loop
	g.Go(func() error {
		echoInterval := l.sess.opts.echoInterval
		tickStep := min(echoInterval, 2*time.Second)
		ticker := time.NewTicker(tickStep)
		defer ticker.Stop()

		failures := 0
		lastTick := time.Now()
		lastEcho := lastTick
		for {
			select {
			case <-gCtx.Done():
				return nil
			case <-l.doneCh:
				return nil
			case now := <-ticker.C:
				woke := detectWakeJump(lastTick, now, tickStep, wakeJumpThreshold)
				lastTick = now
				if !woke && now.Sub(lastEcho) < echoInterval {
					continue
				}
				lastEcho = now

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
				recovered := failures > 0
				failures = 0
				ms := rtt.Milliseconds()
				if ms < 1 && rtt > 0 {
					ms = 1
				}
				l.sess.rttMillis.Store(ms)
				if woke || recovered {
					l.sess.ensureDNS(gCtx)
				}
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
