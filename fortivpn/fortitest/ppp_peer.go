package fortitest

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"sync"

	"tunnelkit/fortivpn/internal/ppp"
)

// gatewayPPPPeer implements the gateway side of an LCP + IPCP negotiation for in-process tests.
type gatewayPPPPeer struct {
	mu       sync.Mutex
	clientIP netip.Addr
	serverIP netip.Addr
	dns      []netip.Addr
	mru      uint16
	dropEcho bool
	send     func(frame []byte)

	closed     bool
	seq        byte
	lcpReqID   byte
	lcpLocal   bool
	lcpRemote  bool
	ipcpReqID  byte
	ipcpLocal  bool
	ipcpRemote bool
}

func newGatewayPPPPeer(clientIP, serverIP netip.Addr, dns []netip.Addr, mru uint16, dropEcho bool, send func([]byte)) *gatewayPPPPeer {
	if !serverIP.IsValid() {
		serverIP = netip.MustParseAddr("169.254.2.1")
	}
	if mru == 0 {
		mru = ppp.DefaultMRU
	}
	return &gatewayPPPPeer{
		clientIP: clientIP,
		serverIP: serverIP,
		dns:      dns,
		mru:      mru,
		dropEcho: dropEcho,
		send:     send,
	}
}

func (p *gatewayPPPPeer) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.sendLCPRequestLocked()
	}
}

func (p *gatewayPPPPeer) setDropEcho(drop bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dropEcho = drop
}

func (p *gatewayPPPPeer) input(frame []byte) {
	proto, payload, ok := ppp.DecodeFrame(frame)
	if !ok {
		return
	}
	pkt, ok := ppp.ParsePacket(payload)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	switch proto {
	case ppp.ProtocolLCP:
		p.onLCPLocked(pkt)
	case ppp.ProtocolIPCP:
		p.onIPCPLocked(pkt)
	}
}

func (p *gatewayPPPPeer) onLCPLocked(pkt ppp.Packet) {
	switch pkt.Code {
	case ppp.CodeConfigureRequest:
		opts, ok := ppp.ParseOptions(pkt.Body)
		if !ok {
			return
		}
		var rejected []ppp.Option
		for _, o := range opts {
			switch o.Type {
			case ppp.OptMRU, ppp.OptMagic, ppp.OptQuality, ppp.OptPFC, ppp.OptACFC:
			default:
				rejected = append(rejected, o)
			}
		}
		if len(rejected) > 0 {
			p.emitLocked(ppp.ProtocolLCP, ppp.Packet{
				Code: ppp.CodeConfigureReject,
				ID:   pkt.ID,
				Body: ppp.MarshalOptions(rejected),
			})
			return
		}
		wasRemote := p.lcpRemote
		p.lcpRemote = true
		p.emitLocked(ppp.ProtocolLCP, ppp.Packet{
			Code: ppp.CodeConfigureAck,
			ID:   pkt.ID,
			Body: pkt.Body,
		})
		if wasRemote && !p.lcpLocal {
			p.sendLCPRequestLocked()
		}
		if p.lcpLocal && p.lcpRemote {
			p.sendIPCPRequestLocked()
		}
	case ppp.CodeConfigureAck:
		if pkt.ID == p.lcpReqID {
			p.lcpLocal = true
			if p.lcpRemote {
				p.sendIPCPRequestLocked()
			}
		}
	case ppp.CodeConfigureNak, ppp.CodeConfigureReject:
		p.sendLCPRequestLocked()
	case ppp.CodeTerminateRequest:
		p.emitLocked(ppp.ProtocolLCP, ppp.Packet{Code: ppp.CodeTerminateAck, ID: pkt.ID})
		p.closed = true
	case ppp.CodeEchoRequest:
		if p.dropEcho {
			return
		}
		reply := []byte{0x54, 0x4b, 0x49, 0x54}
		if len(pkt.Body) > 4 {
			reply = append(reply, pkt.Body[4:]...)
		}
		p.emitLocked(ppp.ProtocolLCP, ppp.Packet{Code: ppp.CodeEchoReply, ID: pkt.ID, Body: reply})
	}
}

func (p *gatewayPPPPeer) onIPCPLocked(pkt ppp.Packet) {
	switch pkt.Code {
	case ppp.CodeConfigureRequest:
		opts, ok := ppp.ParseOptions(pkt.Body)
		if !ok {
			return
		}
		client4 := p.clientIP.As4()
		dns1 := p.dnsOctets(0)
		dns2 := p.dnsOctets(1)
		var nak []ppp.Option
		for _, o := range opts {
			switch o.Type {
			case ppp.OptIPAddress:
				if !bytes.Equal(o.Value, client4[:]) {
					nak = append(nak, ppp.Option{Type: ppp.OptIPAddress, Value: client4[:]})
				}
			case ppp.OptPrimaryDNS:
				if !bytes.Equal(o.Value, dns1[:]) {
					nak = append(nak, ppp.Option{Type: ppp.OptPrimaryDNS, Value: dns1[:]})
				}
			case ppp.OptSecondaryDNS:
				if !bytes.Equal(o.Value, dns2[:]) {
					nak = append(nak, ppp.Option{Type: ppp.OptSecondaryDNS, Value: dns2[:]})
				}
			}
		}
		if len(nak) > 0 {
			p.emitLocked(ppp.ProtocolIPCP, ppp.Packet{
				Code: ppp.CodeConfigureNak,
				ID:   pkt.ID,
				Body: ppp.MarshalOptions(nak),
			})
			return
		}
		wasRemote := p.ipcpRemote
		p.ipcpRemote = true
		p.emitLocked(ppp.ProtocolIPCP, ppp.Packet{
			Code: ppp.CodeConfigureAck,
			ID:   pkt.ID,
			Body: pkt.Body,
		})
		if wasRemote && !p.ipcpLocal {
			p.sendIPCPRequestLocked()
		}
	case ppp.CodeConfigureAck:
		if pkt.ID == p.ipcpReqID {
			p.ipcpLocal = true
		}
	}
}

func (p *gatewayPPPPeer) sendLCPRequestLocked() {
	var mru [2]byte
	binary.BigEndian.PutUint16(mru[:], p.mru)
	p.seq++
	p.lcpReqID = p.seq
	opts := []ppp.Option{
		{Type: ppp.OptMRU, Value: mru[:]},
		{Type: ppp.OptMagic, Value: []byte{0x54, 0x4b, 0x49, 0x54}},
	}
	p.emitLocked(ppp.ProtocolLCP, ppp.Packet{
		Code: ppp.CodeConfigureRequest,
		ID:   p.lcpReqID,
		Body: ppp.MarshalOptions(opts),
	})
}

func (p *gatewayPPPPeer) sendIPCPRequestLocked() {
	srv4 := p.serverIP.As4()
	p.seq++
	p.ipcpReqID = p.seq
	opts := []ppp.Option{{Type: ppp.OptIPAddress, Value: srv4[:]}}
	p.emitLocked(ppp.ProtocolIPCP, ppp.Packet{
		Code: ppp.CodeConfigureRequest,
		ID:   p.ipcpReqID,
		Body: ppp.MarshalOptions(opts),
	})
}

func (p *gatewayPPPPeer) dnsOctets(index int) [4]byte {
	if index < len(p.dns) && p.dns[index].IsValid() && p.dns[index].Is4() {
		return p.dns[index].As4()
	}
	return [4]byte{0, 0, 0, 0}
}

func (p *gatewayPPPPeer) emitLocked(proto uint16, pkt ppp.Packet) {
	if p.send != nil {
		p.send(ppp.EncodeFrame(proto, pkt.Marshal()))
	}
}
