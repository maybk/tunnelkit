package ppp

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// testPeer is a minimal in-package PPP peer simulator used exclusively by unit tests.
type testPeer struct {
	mu       sync.Mutex
	clientIP netip.Addr
	serverIP netip.Addr
	dns      []netip.Addr
	mru      uint16
	send     func([]byte)

	seq       byte
	lcpReqID  byte
	lcpAcked  bool
	lcpPeerOK bool
	ipcpReqID byte
}

func newTestPeer(clientIP, serverIP netip.Addr, mru uint16, send func([]byte)) *testPeer {
	return &testPeer{
		clientIP: clientIP,
		serverIP: serverIP,
		mru:      mru,
		send:     send,
	}
}

func (p *testPeer) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendLCPReqLocked()
}

func (p *testPeer) sendLCPReqLocked() {
	p.seq++
	p.lcpReqID = p.seq
	var mru [2]byte
	binary.BigEndian.PutUint16(mru[:], p.mru)
	opts := []Option{
		{Type: OptMRU, Value: mru[:]},
		{Type: OptMagic, Value: []byte{0xde, 0xad, 0xbe, 0xef}},
	}
	p.emitLocked(ProtocolLCP, Packet{Code: CodeConfigureRequest, ID: p.lcpReqID, Body: MarshalOptions(opts)})
}

func (p *testPeer) sendIPCPReqLocked() {
	p.seq++
	p.ipcpReqID = p.seq
	srv := p.serverIP.As4()
	opts := []Option{{Type: OptIPAddress, Value: srv[:]}}
	p.emitLocked(ProtocolIPCP, Packet{Code: CodeConfigureRequest, ID: p.ipcpReqID, Body: MarshalOptions(opts)})
}

func (p *testPeer) emitLocked(proto uint16, pkt Packet) {
	if p.send != nil {
		p.send(EncodeFrame(proto, pkt.Marshal()))
	}
}

func (p *testPeer) input(frame []byte) {
	proto, payload, ok := DecodeFrame(frame)
	if !ok {
		return
	}
	pkt, ok := ParsePacket(payload)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	switch proto {
	case ProtocolLCP:
		switch pkt.Code {
		case CodeConfigureRequest:
			retransmitted := p.lcpPeerOK
			p.lcpPeerOK = true
			p.emitLocked(ProtocolLCP, Packet{Code: CodeConfigureAck, ID: pkt.ID, Body: pkt.Body})
			if retransmitted && !p.lcpAcked {
				p.sendLCPReqLocked()
			}
			if p.lcpAcked && p.lcpPeerOK {
				p.sendIPCPReqLocked()
			}
		case CodeConfigureAck:
			if pkt.ID == p.lcpReqID {
				p.lcpAcked = true
				if p.lcpPeerOK {
					p.sendIPCPReqLocked()
				}
			}
		case CodeEchoRequest:
			replyBody := append([]byte{0xde, 0xad, 0xbe, 0xef}, pkt.Body[min(4, len(pkt.Body)):]...)
			p.emitLocked(ProtocolLCP, Packet{Code: CodeEchoReply, ID: pkt.ID, Body: replyBody})
		case CodeTerminateRequest:
			p.emitLocked(ProtocolLCP, Packet{Code: CodeTerminateAck, ID: pkt.ID})
		}
	case ProtocolIPCP:
		if pkt.Code == CodeConfigureRequest {
			opts, ok := ParseOptions(pkt.Body)
			if !ok {
				return
			}
			wantIP := p.clientIP.As4()
			var nak []Option
			for _, o := range opts {
				switch o.Type {
				case OptIPAddress:
					if !bytes.Equal(o.Value, wantIP[:]) {
						nak = append(nak, Option{Type: OptIPAddress, Value: wantIP[:]})
					}
				case OptPrimaryDNS, OptSecondaryDNS:
					if !bytes.Equal(o.Value, []byte{0, 0, 0, 0}) {
						nak = append(nak, Option{Type: o.Type, Value: []byte{0, 0, 0, 0}})
					}
				}
			}
			if len(nak) > 0 {
				p.emitLocked(ProtocolIPCP, Packet{Code: CodeConfigureNak, ID: pkt.ID, Body: MarshalOptions(nak)})
				return
			}
			p.emitLocked(ProtocolIPCP, Packet{Code: CodeConfigureAck, ID: pkt.ID, Body: pkt.Body})
		}
	}
}

func TestClientServerHandshakeAndEcho(t *testing.T) {
	var client *Session
	var peer *testPeer
	var queue []struct {
		toPeer bool
		frame  []byte
	}

	var gotCfg IPConfig
	var gotMRU int
	var gotNS1, gotNS2 string
	var up bool

	cfg := DefaultFSMConfig()
	cfg.RestartInterval = 50 * time.Millisecond

	client = NewSession(Events{
		Send: func(frame []byte) {
			queue = append(queue, struct {
				toPeer bool
				frame  []byte
			}{toPeer: true, frame: append([]byte(nil), frame...)})
		},
		NetworkUp: func(c IPConfig, mru int, ns1, ns2 string) {
			up = true
			gotCfg = c
			gotMRU = mru
			gotNS1, gotNS2 = ns1, ns2
		},
	}, cfg)

	peer = newTestPeer(
		netip.MustParseAddr("10.212.0.10"),
		netip.MustParseAddr("169.254.2.1"),
		1400,
		func(frame []byte) {
			queue = append(queue, struct {
				toPeer bool
				frame  []byte
			}{toPeer: false, frame: append([]byte(nil), frame...)})
		},
	)

	client.Start()
	peer.start()

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.toPeer {
			peer.input(item.frame)
		} else {
			client.Input(item.frame)
		}
	}

	if !up {
		t.Fatal("PPP session did not reach NetworkUp")
	}
	if gotCfg.LocalIP != netip.MustParseAddr("10.212.0.10") {
		t.Fatalf("LocalIP = %v, want 10.212.0.10", gotCfg.LocalIP)
	}
	if gotCfg.PeerIP != netip.MustParseAddr("169.254.2.1") {
		t.Fatalf("PeerIP = %v, want 169.254.2.1", gotCfg.PeerIP)
	}
	if len(gotCfg.DNS) != 0 {
		t.Fatalf("DNS = %v, want empty when gateway returns 0.0.0.0", gotCfg.DNS)
	}
	if gotNS1 != "0.0.0.0" || gotNS2 != "0.0.0.0" {
		t.Fatalf("RawDNSStrings = (%q, %q), want (0.0.0.0, 0.0.0.0)", gotNS1, gotNS2)
	}
	if gotMRU != 1400 {
		t.Fatalf("MRU = %d, want 1400", gotMRU)
	}

	client.ev.Send = func(frame []byte) {
		go peer.input(append([]byte(nil), frame...))
	}
	peer.mu.Lock()
	peer.send = func(frame []byte) {
		go client.Input(append([]byte(nil), frame...))
	}
	peer.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rtt, err := client.SendEcho(ctx)
	if err != nil {
		t.Fatalf("SendEcho: %v", err)
	}
	if rtt < 0 {
		t.Fatalf("negative RTT: %v", rtt)
	}

	if err := client.Terminate(ctx); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
}

func TestPeerResendsLCPOnClientRetransmit(t *testing.T) {
	toPeerCh := make(chan []byte, 32)
	toClientCh := make(chan []byte, 32)
	doneCh := make(chan struct{})
	defer close(doneCh)

	upCh := make(chan struct{}, 1)

	cfg := DefaultFSMConfig()
	cfg.RestartInterval = 15 * time.Millisecond

	client := NewSession(Events{
		Send: func(frame []byte) {
			cp := append([]byte(nil), frame...)
			select {
			case toPeerCh <- cp:
			case <-doneCh:
			}
		},
		NetworkUp: func(IPConfig, int, string, string) {
			select {
			case upCh <- struct{}{}:
			default:
			}
		},
	}, cfg)

	dropFirstPeerFrame := true
	peer := newTestPeer(
		netip.MustParseAddr("10.212.0.10"),
		netip.MustParseAddr("169.254.2.1"),
		1400,
		func(frame []byte) {
			if dropFirstPeerFrame {
				dropFirstPeerFrame = false
				return
			}
			cp := append([]byte(nil), frame...)
			select {
			case toClientCh <- cp:
			case <-doneCh:
			}
		},
	)

	go func() {
		for {
			select {
			case f := <-toPeerCh:
				peer.input(f)
			case <-doneCh:
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case f := <-toClientCh:
				client.Input(f)
			case <-doneCh:
				return
			}
		}
	}()

	peer.start() // Initial peer LCP Configure-Request is dropped.
	client.Start()
	defer client.Close()

	select {
	case <-upCh:
	case <-time.After(time.Second):
		t.Fatal("PPP session did not recover when initial peer LCP Configure-Request was dropped")
	}
}

func TestFSMTimeoutAndProtocolReject(t *testing.T) {
	var sent [][]byte
	var finishedErr error
	cfg := FSMConfig{
		RestartInterval: 10 * time.Millisecond,
		MaxConfigure:    3,
		MaxTerminate:    2,
		MaxFailure:      2,
	}

	s := NewSession(Events{
		Send: func(frame []byte) {
			sent = append(sent, append([]byte(nil), frame...))
		},
		Closed: func(err error) {
			finishedErr = err
		},
	}, cfg)

	s.Start()
	time.Sleep(80 * time.Millisecond)

	s.mu.Lock()
	err := finishedErr
	sentCount := len(sent)
	s.mu.Unlock()

	if err == nil {
		t.Fatal("expected timeout error after MaxConfigure attempts")
	}
	if sentCount != 3 {
		t.Fatalf("sent %d Configure-Requests, want 3", sentCount)
	}
}

func TestProtocolRejectOnCCP(t *testing.T) {
	var lastFrame []byte
	s := NewSession(Events{
		Send: func(frame []byte) {
			lastFrame = append([]byte(nil), frame...)
		},
	}, DefaultFSMConfig())

	s.Start()
	_, lcpPayload, _ := DecodeFrame(lastFrame)
	reqPkt, _ := ParsePacket(lcpPayload)
	s.Input(EncodeFrame(ProtocolLCP, Packet{Code: CodeConfigureAck, ID: reqPkt.ID, Body: reqPkt.Body}.Marshal()))
	s.Input(EncodeFrame(ProtocolLCP, Packet{Code: CodeConfigureRequest, ID: 7, Body: nil}.Marshal()))

	s.Input(EncodeFrame(ProtocolCCP, []byte{1, 1, 0, 4}))
	proto, payload, ok := DecodeFrame(lastFrame)
	if !ok || proto != ProtocolLCP {
		t.Fatalf("expected LCP frame, got proto %#x", proto)
	}
	pkt, ok := ParsePacket(payload)
	if !ok || pkt.Code != CodeProtocolReject {
		t.Fatalf("expected CodeProtocolReject, got code %d", pkt.Code)
	}
	if len(pkt.Body) < 2 || binary.BigEndian.Uint16(pkt.Body[:2]) != ProtocolCCP {
		t.Fatalf("Protocol-Reject body = %x, want rejected protocol %#x", pkt.Body, ProtocolCCP)
	}
	s.Close()
}

func TestIPCPAcceptsPeerDNSAndLCPRejectsIPCP(t *testing.T) {
	h := NewIPCPHandler()
	verdict, _, err := h.EvaluatePeerRequest([]Option{
		{Type: OptIPAddress, Value: []byte{169, 254, 2, 1}},
		{Type: OptPrimaryDNS, Value: []byte{10, 20, 0, 53}},
		{Type: OptSecondaryDNS, Value: []byte{10, 20, 0, 54}},
	})
	if err != nil || verdict != PeerAck {
		t.Fatalf("EvaluatePeerRequest = (%v, %v), want PeerAck", verdict, err)
	}
	cfg := h.Config()
	if len(cfg.DNS) != 2 || cfg.DNS[0] != netip.MustParseAddr("10.20.0.53") || cfg.DNS[1] != netip.MustParseAddr("10.20.0.54") {
		t.Fatalf("IPCP Config DNS = %v, want [10.20.0.53 10.20.0.54]", cfg.DNS)
	}

	var closedErr error
	s := NewSession(Events{
		Send:   func([]byte) {},
		Closed: func(e error) { closedErr = e },
	}, DefaultFSMConfig())
	s.Start()
	var body [2]byte
	binary.BigEndian.PutUint16(body[:], ProtocolIPCP)
	s.Input(EncodeFrame(ProtocolLCP, Packet{Code: CodeProtocolReject, ID: 1, Body: body[:]}.Marshal()))
	if closedErr == nil {
		t.Fatal("expected session to fail fast when peer sends Protocol-Reject for IPCP")
	}
}

func TestFSMNakLoopCapAndFallbackLocalIP(t *testing.T) {
	h := NewIPCPHandler()
	h.SetFallbackIP(netip.MustParseAddr("10.200.0.77"))
	if got := h.Config().LocalIP; got != netip.MustParseAddr("10.200.0.77") {
		t.Fatalf("LocalIP with fallback = %v, want 10.200.0.77", got)
	}
	_ = h.ApplyNak([]Option{{Type: OptIPAddress, Value: []byte{0, 0, 0, 0}}})
	if got := h.Config().LocalIP; got != netip.MustParseAddr("10.200.0.77") {
		t.Fatalf("LocalIP after 0.0.0.0 Nak = %v, want 10.200.0.77", got)
	}

	var lastSent Packet
	var failedErr error
	fsm := NewFSM(NewIPCPHandler(), FSMCallbacks{
		Send: func(_ uint16, payload []byte) {
			lastSent, _ = ParsePacket(payload)
		},
		ThisLayerFinished: func(err error) {
			failedErr = err
		},
	}, FSMConfig{
		RestartInterval: 50 * time.Millisecond,
		MaxConfigure:    3,
		MaxTerminate:    2,
		MaxFailure:      2,
	})
	fsm.Open(nil)
	fsm.Up(nil)
	for i := 0; i < 4; i++ {
		fsm.Input(Packet{
			Code: CodeConfigureNak,
			ID:   lastSent.ID,
			Body: MarshalOptions([]Option{{Type: OptIPAddress, Value: []byte{10, 0, 0, byte(i + 1)}}}),
		}.Marshal(), nil)
	}
	if failedErr == nil {
		t.Fatal("expected FSM to fail after exceeding MaxConfigure Configure-Naks")
	}
}

func TestExtractIPv4AndPaddingTrim(t *testing.T) {
	pkt := make([]byte, 28)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], 28)
	pkt[9] = 1

	padded := append(append([]byte{}, pkt...), 0, 0, 0, 0, 0)
	frame := EncodeFrame(ProtocolIP, padded)
	got, ok := ExtractIPv4(frame)
	if !ok {
		t.Fatal("ExtractIPv4 failed on valid padded IPv4 packet")
	}
	if !bytes.Equal(got, pkt) {
		t.Fatalf("ExtractIPv4 = %x, want %x", got, pkt)
	}

	if _, ok := ExtractIPv4(EncodeFrame(ProtocolIP, []byte{0x45, 0, 0, 50, 1, 2})); ok {
		t.Fatal("ExtractIPv4 accepted truncated IPv4 packet")
	}
}

func TestHotPathZeroAllocations(t *testing.T) {
	lcp := NewLCPHandler(1400, nil)
	ipcp := NewIPCPHandler()
	rawOpts := MarshalOptions(ipcp.BuildRequestOptions())
	buf := make([]byte, HeaderLen+28)
	buf[HeaderLen] = 0x45
	binary.BigEndian.PutUint16(buf[HeaderLen+2:HeaderLen+4], 28)

	allocs := testing.AllocsPerRun(100, func() {
		_ = lcp.BuildRequestOptions()
		_ = ipcp.BuildRequestOptions()
		cur := NewOptionCursor(rawOpts)
		for {
			_, ok := cur.Next()
			if !ok {
				break
			}
		}
		_ = cur.Valid()
		PutFrameHeader(buf, ProtocolIP)
		_, _ = ExtractIPv4(buf)
	})
	if allocs > 0 {
		t.Fatalf("PPP hot-path helpers allocated %.2f times per run, want 0", allocs)
	}
}

func FuzzPPPDecode(f *testing.F) {
	f.Add([]byte{0xff, 0x03, 0xc0, 0x21, 0x01, 0x01, 0x00, 0x08, 0x01, 0x04, 0x05, 0x78})
	f.Add([]byte{0xff, 0x03, 0x00, 0x21, 0x45, 0x00, 0x00, 0x14})
	f.Fuzz(func(t *testing.T, data []byte) {
		proto, payload, ok := DecodeFrame(data)
		if !ok {
			return
		}
		if proto == ProtocolIP {
			_, _ = TrimToIPv4(payload)
			return
		}
		if pkt, ok := ParsePacket(payload); ok {
			_, _ = ParseOptions(pkt.Body)
			cur := NewOptionCursor(pkt.Body)
			for {
				if _, ok := cur.Next(); !ok {
					break
				}
			}
			_ = cur.Valid()
		}
	})
}
