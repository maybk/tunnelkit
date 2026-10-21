package ppp

import (
	"encoding/binary"
	"net/netip"
	"sync"
)

// ServerConfig configures a NoAuth PPP server session (used by fortitest.Gateway).
type ServerConfig struct {
	ClientIP netip.Addr
	ServerIP netip.Addr
	DNS      []netip.Addr
	MRU      uint16
	DropEcho bool
}

// ServerEvents receives callbacks from a ServerSession.
type ServerEvents struct {
	Send      func(frame []byte)
	NetworkUp func()
	Closed    func(err error)
}

// ServerSession is the gateway side of a Fortinet PPP link (adapted from veepin's
// ServerSession in NoAuth mode).
type ServerSession struct {
	cfg ServerConfig
	ev  ServerEvents

	mu             sync.Mutex
	closed         bool
	magic          uint32
	reqID          byte
	lcpReqID       byte
	lcpLocalOpen   bool
	lcpRemoteOpen  bool
	ipcpReqID      byte
	ipcpLocalOpen  bool
	ipcpRemoteOpen bool
}

// NewServerSession constructs a gateway-side PPP session.
func NewServerSession(cfg ServerConfig, ev ServerEvents) *ServerSession {
	if !cfg.ServerIP.IsValid() {
		cfg.ServerIP = netip.MustParseAddr("169.254.2.1")
	}
	if cfg.MRU == 0 {
		cfg.MRU = DefaultMRU
	}
	return &ServerSession{
		cfg:   cfg,
		ev:    ev,
		magic: randomNonZeroMagic(),
	}
}

// Start sends the server's initial LCP Configure-Request.
func (s *ServerSession) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.sendLCPConfigReqLocked()
}

// Input processes one inbound PPP frame from the client.
func (s *ServerSession) Input(frame []byte) {
	proto, payload, ok := DecodeFrame(frame)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	switch proto {
	case ProtocolLCP:
		s.handleLCPLocked(payload)
	case ProtocolIPCP:
		s.handleIPCPLocked(payload)
	}
}

// SetDropEcho toggles whether the server ignores LCP Echo-Requests (for testing peer_dead).
func (s *ServerSession) SetDropEcho(drop bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.DropEcho = drop
}

func (s *ServerSession) sendLCPConfigReqLocked() {
	var mru [2]byte
	binary.BigEndian.PutUint16(mru[:], s.cfg.MRU)
	var magic [4]byte
	binary.BigEndian.PutUint32(magic[:], s.magic)
	opts := []Option{
		{Type: OptMRU, Value: mru[:]},
		{Type: OptMagic, Value: magic[:]},
	}
	s.reqID++
	s.lcpReqID = s.reqID
	s.sendLocked(ProtocolLCP, Packet{
		Code: CodeConfigureRequest,
		ID:   s.lcpReqID,
		Body: MarshalOptions(opts),
	})
}

func (s *ServerSession) handleLCPLocked(payload []byte) {
	pkt, ok := ParsePacket(payload)
	if !ok {
		return
	}
	switch pkt.Code {
	case CodeConfigureRequest:
		opts, ok := ParseOptions(pkt.Body)
		if !ok {
			return
		}
		var rejected []Option
		for _, o := range opts {
			switch o.Type {
			case OptMRU, OptMagic, OptQuality, OptPFC, OptACFC:
			default:
				rejected = append(rejected, o)
			}
		}
		if len(rejected) > 0 {
			s.sendLocked(ProtocolLCP, Packet{
				Code: CodeConfigureReject,
				ID:   pkt.ID,
				Body: MarshalOptions(rejected),
			})
			return
		}
		s.sendLocked(ProtocolLCP, Packet{
			Code: CodeConfigureAck,
			ID:   pkt.ID,
			Body: pkt.Body,
		})
		s.lcpRemoteOpen = true
		s.maybeLCPUpLocked()
	case CodeConfigureAck:
		if pkt.ID == s.lcpReqID {
			s.lcpLocalOpen = true
			s.maybeLCPUpLocked()
		}
	case CodeConfigureNak, CodeConfigureReject:
		s.sendLCPConfigReqLocked()
	case CodeTerminateRequest:
		s.sendLocked(ProtocolLCP, Packet{Code: CodeTerminateAck, ID: pkt.ID})
		s.closed = true
		if s.ev.Closed != nil {
			s.ev.Closed(nil)
		}
	case CodeEchoRequest:
		if s.cfg.DropEcho {
			return
		}
		var magic [4]byte
		binary.BigEndian.PutUint32(magic[:], s.magic)
		body := magic[:]
		if len(pkt.Body) > 4 {
			body = append(magic[:], pkt.Body[4:]...)
		}
		s.sendLocked(ProtocolLCP, Packet{Code: CodeEchoReply, ID: pkt.ID, Body: body})
	}
}

func (s *ServerSession) maybeLCPUpLocked() {
	if s.lcpLocalOpen && s.lcpRemoteOpen {
		s.sendIPCPConfigReqLocked()
	}
}

func (s *ServerSession) sendIPCPConfigReqLocked() {
	srv4 := s.cfg.ServerIP.As4()
	opts := []Option{{Type: OptIPAddress, Value: srv4[:]}}
	s.reqID++
	s.ipcpReqID = s.reqID
	s.sendLocked(ProtocolIPCP, Packet{
		Code: CodeConfigureRequest,
		ID:   s.ipcpReqID,
		Body: MarshalOptions(opts),
	})
}

func (s *ServerSession) handleIPCPLocked(payload []byte) {
	pkt, ok := ParsePacket(payload)
	if !ok {
		return
	}
	switch pkt.Code {
	case CodeConfigureRequest:
		opts, ok := ParseOptions(pkt.Body)
		if !ok {
			return
		}
		var nak []Option
		client4 := s.cfg.ClientIP.As4()
		dns1 := s.dnsAtLocked(0)
		dns2 := s.dnsAtLocked(1)
		for _, o := range opts {
			switch o.Type {
			case OptIPAddress:
				if !bytes4Eq(o.Value, client4) {
					nak = append(nak, Option{Type: OptIPAddress, Value: client4[:]})
				}
			case OptPrimaryDNS:
				if !bytes4Eq(o.Value, dns1) {
					nak = append(nak, Option{Type: OptPrimaryDNS, Value: dns1[:]})
				}
			case OptSecondaryDNS:
				if !bytes4Eq(o.Value, dns2) {
					nak = append(nak, Option{Type: OptSecondaryDNS, Value: dns2[:]})
				}
			}
		}
		if len(nak) > 0 {
			s.sendLocked(ProtocolIPCP, Packet{
				Code: CodeConfigureNak,
				ID:   pkt.ID,
				Body: MarshalOptions(nak),
			})
			return
		}
		s.sendLocked(ProtocolIPCP, Packet{
			Code: CodeConfigureAck,
			ID:   pkt.ID,
			Body: pkt.Body,
		})
		s.ipcpRemoteOpen = true
		s.maybeIPCPUpLocked()
	case CodeConfigureAck:
		if pkt.ID == s.ipcpReqID {
			s.ipcpLocalOpen = true
			s.maybeIPCPUpLocked()
		}
	}
}

func (s *ServerSession) dnsAtLocked(i int) [4]byte {
	if i < len(s.cfg.DNS) && s.cfg.DNS[i].Is4() {
		return s.cfg.DNS[i].As4()
	}
	return [4]byte{0, 0, 0, 0}
}

func (s *ServerSession) maybeIPCPUpLocked() {
	if s.ipcpLocalOpen && s.ipcpRemoteOpen && s.ev.NetworkUp != nil {
		s.ev.NetworkUp()
	}
}

func (s *ServerSession) sendLocked(proto uint16, pkt Packet) {
	if s.ev.Send != nil {
		s.ev.Send(EncodeFrame(proto, pkt.Marshal()))
	}
}

func bytes4Eq(b []byte, want [4]byte) bool {
	return len(b) == 4 && b[0] == want[0] && b[1] == want[1] && b[2] == want[2] && b[3] == want[3]
}
