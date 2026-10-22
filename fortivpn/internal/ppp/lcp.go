package ppp

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
)

// LCP option types (RFC 1661 §6).
const (
	OptMRU       byte = 1
	OptAsyncMap  byte = 2
	OptAuthProto byte = 3
	OptQuality   byte = 4
	OptMagic     byte = 5
	OptPFC       byte = 7
	OptACFC      byte = 8
)

// DefaultMRU is the default Maximum Receive Unit negotiated by LCP.
const DefaultMRU uint16 = 1400

const (
	minMRU          uint16 = 576
	maxLoopbackNaks        = 5
)

// LCPHandler implements OptionHandler for the Link Control Protocol (0xC021).
type LCPHandler struct {
	initialMRU   uint16
	useMRU       bool
	useMagic     bool
	localMRU     uint16
	peerMRU      uint16
	localMagic   uint32
	peerMagic    uint32
	loopbackNaks int
	onEchoReply  func(id byte)
}

// NewLCPHandler creates an LCP option handler with the given initial MRU.
func NewLCPHandler(initialMRU uint16, onEchoReply func(id byte)) *LCPHandler {
	if initialMRU == 0 {
		initialMRU = DefaultMRU
	}
	h := &LCPHandler{
		initialMRU:  initialMRU,
		onEchoReply: onEchoReply,
	}
	h.ResetOptions()
	return h
}

func (h *LCPHandler) Protocol() uint16 { return ProtocolLCP }
func (h *LCPHandler) Name() string     { return "LCP" }

func (h *LCPHandler) ResetOptions() {
	h.useMRU = true
	h.useMagic = true
	h.localMRU = h.initialMRU
	h.peerMRU = h.initialMRU
	h.localMagic = randomNonZeroMagic()
	h.peerMagic = 0
	h.loopbackNaks = 0
}

// NegotiatedMRU returns the effective MTU/MRU after LCP reaches Opened.
func (h *LCPHandler) NegotiatedMRU() int {
	mru := h.localMRU
	if h.peerMRU > 0 && h.peerMRU < mru {
		mru = h.peerMRU
	}
	if mru < minMRU {
		mru = DefaultMRU
	}
	return int(mru)
}

// LocalMagic returns our current 32-bit magic number.
func (h *LCPHandler) LocalMagic() uint32 { return h.localMagic }

func (h *LCPHandler) BuildRequestOptions() []Option {
	var opts []Option
	if h.useMRU {
		var mru [2]byte
		binary.BigEndian.PutUint16(mru[:], h.localMRU)
		opts = append(opts, Option{Type: OptMRU, Value: mru[:]})
	}
	if h.useMagic {
		var magic [4]byte
		binary.BigEndian.PutUint32(magic[:], h.localMagic)
		opts = append(opts, Option{Type: OptMagic, Value: magic[:]})
	}
	return opts
}

func (h *LCPHandler) ApplyNak(opts []Option) error {
	for _, o := range opts {
		switch o.Type {
		case OptMRU:
			if len(o.Value) == 2 {
				suggested := binary.BigEndian.Uint16(o.Value)
				if suggested >= minMRU && suggested <= 16384 {
					h.localMRU = suggested
				} else {
					h.useMRU = false
				}
			}
		case OptMagic:
			if len(o.Value) == 4 {
				suggested := binary.BigEndian.Uint32(o.Value)
				if suggested == h.localMagic {
					h.loopbackNaks++
					if h.loopbackNaks >= maxLoopbackNaks {
						return fmt.Errorf("LCP magic-number loopback detected")
					}
					h.localMagic = randomNonZeroMagic()
				} else if suggested != 0 {
					h.localMagic = suggested
				} else {
					h.localMagic = randomNonZeroMagic()
				}
			}
		}
	}
	return nil
}

func (h *LCPHandler) ApplyReject(opts []Option) error {
	for _, o := range opts {
		switch o.Type {
		case OptMRU:
			h.useMRU = false
		case OptMagic:
			h.useMagic = false
		}
	}
	return nil
}

func (h *LCPHandler) EvaluatePeerRequest(opts []Option) (PeerEvaluation, []Option, error) {
	var rejected, naked []Option
	for _, o := range opts {
		switch o.Type {
		case OptMRU:
			if len(o.Value) != 2 {
				rejected = append(rejected, o)
				continue
			}
			mru := binary.BigEndian.Uint16(o.Value)
			if mru < minMRU {
				var want [2]byte
				binary.BigEndian.PutUint16(want[:], DefaultMRU)
				naked = append(naked, Option{Type: OptMRU, Value: want[:]})
				continue
			}
			h.peerMRU = mru
		case OptMagic:
			if len(o.Value) != 4 {
				rejected = append(rejected, o)
				continue
			}
			peerMagic := binary.BigEndian.Uint32(o.Value)
			if h.useMagic && peerMagic != 0 && peerMagic == h.localMagic {
				h.loopbackNaks++
				if h.loopbackNaks >= maxLoopbackNaks {
					return PeerReject, nil, fmt.Errorf("LCP magic-number loopback detected")
				}
				h.localMagic = randomNonZeroMagic()
				var newMagic [4]byte
				binary.BigEndian.PutUint32(newMagic[:], h.localMagic)
				naked = append(naked, Option{Type: OptMagic, Value: newMagic[:]})
				continue
			}
			h.peerMagic = peerMagic
		case OptAsyncMap, OptQuality, OptPFC, OptACFC:
			// Acceptable without requiring us to compress outbound frames.
		default:
			// Includes OptAuthProto: Fortinet authenticates at the HTTPS layer.
			rejected = append(rejected, o)
		}
	}

	if len(rejected) > 0 {
		return PeerReject, rejected, nil
	}
	if len(naked) > 0 {
		return PeerNak, naked, nil
	}
	return PeerAck, opts, nil
}

func (h *LCPHandler) HandleExtraCode(pkt Packet, send func(Packet)) (bool, error) {
	switch pkt.Code {
	case CodeEchoRequest:
		var magic [4]byte
		binary.BigEndian.PutUint32(magic[:], h.localMagic)
		body := magic[:]
		if len(pkt.Body) > 4 {
			body = append(magic[:], pkt.Body[4:]...)
		}
		send(Packet{Code: CodeEchoReply, ID: pkt.ID, Body: body})
		return true, nil
	case CodeEchoReply:
		if h.onEchoReply != nil {
			h.onEchoReply(pkt.ID)
		}
		return true, nil
	case CodeDiscardRequest:
		return true, nil
	case CodeProtocolReject:
		if len(pkt.Body) >= 2 && binary.BigEndian.Uint16(pkt.Body[:2]) == ProtocolIPCP {
			return true, fmt.Errorf("LCP peer rejected IPCP protocol (0x8021)")
		}
		return true, nil
	default:
		return false, nil
	}
}

func randomNonZeroMagic() uint32 {
	var b [4]byte
	for {
		_, _ = rand.Read(b[:])
		v := binary.BigEndian.Uint32(b[:])
		if v != 0 {
			return v
		}
	}
}
