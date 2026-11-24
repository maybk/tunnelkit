package engine

import (
	"context"
	"errors"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/netdev"
	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

const wakeJumpThreshold = 15 * time.Second

type dnsMaintainer interface {
	EnsureDNS(ctx context.Context) error
}

func (s *session) deviceReaderBatchLoop(bdev batchDevice) {
	batchSize := max(bdev.BatchSize(), 1)
	ptrs := make([]*[]byte, batchSize)
	bufs := make([][]byte, batchSize)
	sizes := make([]int, batchSize)
	defer func() {
		for i, ptr := range ptrs {
			if ptr != nil {
				putDataFrameBuf(ptr)
				ptrs[i] = nil
			}
		}
	}()

	for {
		for i := range batchSize {
			if ptrs[i] == nil {
				ptr := getDataFrameBuf(maxIPv4PacketSize)
				ptrs[i] = ptr
				bufs[i] = (*ptr)[:cap(*ptr)]
			}
		}

		n, err := bdev.ReadBatch(bufs, sizes, outboundIPOffset)
		if n > 0 {
			lnk := s.activeLink.Load()
			for i := range n {
				ptr := ptrs[i]
				ptrs[i] = nil
				bufs[i] = nil
				if sizes[i] <= 0 || lnk == nil {
					putDataFrameBuf(ptr)
					continue
				}
				lnk.enqueueDataHeadroom(ptr, sizes[i])
			}
		}
		if err != nil {
			return
		}
	}
}

// writeCoalescedData sends firstPtr immediately with zero added latency. When
// additional outbound packets are already queued in dataCh (and no PPP control
// frame is waiting), it coalesces consecutive preframed 0x5050 records up to
// one 16 KiB TLS record. If a dequeued packet does not fit in the current
// record, it is returned as overflow to seed the next batch without emitting an
// isolated single-packet TLS record.
func (l *link) writeCoalescedData(firstPtr *[]byte, coalesceBuf []byte) (*[]byte, error) {
	firstBuf := *firstPtr
	firstFramed := firstBuf[outboundFrameOffset:]
	firstPayload := uint64(max(len(firstBuf)-outboundIPOffset, 0))

	if len(l.ctrlCh) > 0 || len(l.dataCh) == 0 || len(firstFramed) >= len(coalesceBuf) {
		err := l.writePreframedBatch(firstFramed, firstPayload, 1)
		putDataFrameBuf(firstPtr)
		return nil, err
	}

	used := copy(coalesceBuf, firstFramed)
	putDataFrameBuf(firstPtr)
	txBytes := firstPayload
	txPackets := uint64(1)

	for len(l.ctrlCh) == 0 && !l.isClosing() {
		select {
		case nextPtr := <-l.dataCh:
			nextBuf := *nextPtr
			nextFramed := nextBuf[outboundFrameOffset:]
			nextPayload := uint64(max(len(nextBuf)-outboundIPOffset, 0))
			if used+len(nextFramed) <= len(coalesceBuf) {
				used += copy(coalesceBuf[used:], nextFramed)
				txBytes += nextPayload
				txPackets++
				putDataFrameBuf(nextPtr)
				continue
			}
			if err := l.writePreframedBatch(coalesceBuf[:used], txBytes, txPackets); err != nil {
				putDataFrameBuf(nextPtr)
				return nil, err
			}
			return nextPtr, nil
		default:
			return nil, l.writePreframedBatch(coalesceBuf[:used], txBytes, txPackets)
		}
	}
	return nil, l.writePreframedBatch(coalesceBuf[:used], txBytes, txPackets)
}

func (l *link) writePreframedBatch(framed []byte, payloadBytes, packets uint64) error {
	if err := l.tconn.WritePreframed(framed); err != nil {
		if l.isClosing() {
			return nil
		}
		return fortivpn.WrapError(fortivpn.CodeConnectionLost, "tunnel TLS write failed", err)
	}
	if payloadBytes > 0 {
		l.sess.txBytes.Add(payloadBytes)
	}
	if packets > 0 {
		l.sess.txPackets.Add(packets)
	}
	return nil
}

// inboundBatchWriter accumulates consecutive IPv4 packets that are already
// decrypted in TunnelConn's bufio.Reader and delivers them via WriteBatch so
// Linux wireguard/tun can apply TCP/UDP GRO coalescing.
type inboundBatchWriter struct {
	limit      int
	slots      [][]byte
	pending    [][]byte
	pendingOff int
	count      int
}

func newInboundBatchWriter(batchSize int) *inboundBatchWriter {
	limit := min(max(batchSize, 1), maxInboundWriteBatch)
	slots := make([][]byte, limit)
	for i := range slots {
		slots[i] = make([]byte, inboundBatchBufCap)
	}
	return &inboundBatchWriter{
		limit:   limit,
		slots:   slots,
		pending: make([][]byte, 0, limit),
	}
}

func (w *inboundBatchWriter) readScratch() []byte {
	return w.slots[w.count][netdev.Headroom:]
}

func (w *inboundBatchWriter) add(frame, ipPkt []byte) (needFlushFirst bool, offset int) {
	pHdrLen := cap(frame) - cap(ipPkt)
	if cap(frame) != cap(w.readScratch()) || pHdrLen < 0 || pHdrLen > ppp.HeaderLen {
		return true, -1
	}
	off := netdev.Headroom + pHdrLen
	if w.count > 0 && off != w.pendingOff {
		return true, off
	}
	slot := w.slots[w.count]
	w.pendingOff = off
	w.pending = append(w.pending, slot[:off+len(ipPkt)])
	w.count++
	return false, off
}

func (w *inboundBatchWriter) flush(dev batchDevice) error {
	if w.count == 0 {
		return nil
	}
	_, err := dev.WriteBatch(w.pending[:w.count], w.pendingOff)
	w.pending = w.pending[:0]
	w.count = 0
	return err
}

func (l *link) writeSingleInbound(dev Device, buf, readScratch, frame, ipPkt []byte) error {
	pHdrLen := cap(frame) - cap(ipPkt)
	var err error
	if fastDev, ok := dev.(headroomDevice); ok && cap(frame) == cap(readScratch) && pHdrLen >= 0 && pHdrLen <= ppp.HeaderLen {
		offset := netdev.Headroom + pHdrLen
		err = fastDev.WritePacketAt(buf[:offset+len(ipPkt)], offset)
	} else {
		err = dev.WritePacket(ipPkt)
	}
	return l.classifyInboundWriteErr(err)
}

func (l *link) classifyInboundWriteErr(err error) error {
	if err == nil || errors.Is(err, netdev.ErrPacketRejected) || l.isClosing() {
		return nil
	}
	return fortivpn.WrapError(fortivpn.CodeDeviceFailed, "TUN device write failed", err)
}

func (l *link) canContinueInboundBatch(w *inboundBatchWriter) bool {
	return w.count > 0 && w.count < w.limit && l.tconn.Buffered() >= sslvpn.FrameHeaderLen && !l.isClosing()
}

// detectWakeJump detects system sleep/wake across macOS, Linux, and Windows (both
// S3 where monotonic clocks pause and Windows S0 Modern Standby where monotonic
// clocks continue advancing while timers suspend).
func detectWakeJump(prev, now time.Time, expectedTick, threshold time.Duration) bool {
	if prev.IsZero() || threshold <= 0 {
		return false
	}
	return isWakeJump(now.Sub(prev), now.Round(0).Sub(prev.Round(0)), expectedTick, threshold)
}

func isWakeJump(monoElapsed, wallElapsed, expectedTick, threshold time.Duration) bool {
	if threshold <= 0 {
		return false
	}
	if wallElapsed-monoElapsed >= threshold {
		return true
	}
	return expectedTick > 0 && monoElapsed >= expectedTick+threshold
}

func (s *session) ensureDNS(ctx context.Context) {
	s.mu.Lock()
	applied := s.applied
	s.mu.Unlock()
	if applied == nil {
		return
	}
	maintainer, ok := applied.(dnsMaintainer)
	if !ok {
		return
	}
	dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := maintainer.EnsureDNS(dnsCtx); err != nil {
		s.log.Warn("DNS reconciliation after wake failed.", "error", err)
	}
}
