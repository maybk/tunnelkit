package engine

import (
	"sync"

	"tunnelkit/fortivpn/internal/netdev"
	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

const (
	outboundIPOffset    = netdev.Headroom
	outboundPPPOffset   = outboundIPOffset - ppp.HeaderLen
	outboundFrameOffset = outboundPPPOffset - sslvpn.FrameHeaderLen
	smallPacketSize     = 2048
	maxIPv4PacketSize   = 65535
	pooledDataBufCap    = outboundIPOffset + smallPacketSize
	pooledPacketBufCap  = outboundIPOffset + maxIPv4PacketSize
)

// headroomDevice permits in-place encapsulation without an engine payload copy.
type headroomDevice interface {
	ReadPacketAt(buf []byte, offset int) (int, error)
	WritePacketAt(buf []byte, offset int) error
}

var packetBufPool = sync.Pool{New: func() any {
	buf := make([]byte, pooledPacketBufCap)
	return &buf
}}

var dataFramePool = sync.Pool{New: func() any {
	buf := make([]byte, pooledDataBufCap)
	return &buf
}}

func getDataFrameBuf(payloadSize int) *[]byte {
	if payloadSize > smallPacketSize {
		return packetBufPool.Get().(*[]byte)
	}
	return dataFramePool.Get().(*[]byte)
}

func putDataFrameBuf(bufPtr *[]byte) {
	if bufPtr == nil {
		return
	}
	buf := *bufPtr
	*bufPtr = buf[:cap(buf)]
	switch cap(buf) {
	case pooledDataBufCap:
		dataFramePool.Put(bufPtr)
	case pooledPacketBufCap:
		packetBufPool.Put(bufPtr)
	}
}
