package ppp

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/netip"
	"testing"
	"time"
)

func TestClientServerHandshakeAndEcho(t *testing.T) {
	var client *Session
	var server *ServerSession
	var queue []struct {
		toServer bool
		frame    []byte
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
				toServer bool
				frame    []byte
			}{toServer: true, frame: append([]byte(nil), frame...)})
		},
		NetworkUp: func(c IPConfig, mru int, ns1, ns2 string) {
			up = true
			gotCfg = c
			gotMRU = mru
			gotNS1, gotNS2 = ns1, ns2
		},
	}, cfg)

	server = NewServerSession(ServerConfig{
		ClientIP: netip.MustParseAddr("10.212.0.10"),
		ServerIP: netip.MustParseAddr("169.254.2.1"),
		DNS:      nil, // simulates gateway returning 0.0.0.0 for IPCP DNS
		MRU:      1400,
	}, ServerEvents{
		Send: func(frame []byte) {
			queue = append(queue, struct {
				toServer bool
				frame    []byte
			}{toServer: false, frame: append([]byte(nil), frame...)})
		},
	})

	client.Start()
	server.Start()

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.toServer {
			server.Input(item.frame)
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

	// Test Echo-Request / Echo-Reply using direct delivery.
	client.ev.Send = func(frame []byte) {
		go server.Input(append([]byte(nil), frame...))
	}
	server.ev.Send = func(frame []byte) {
		go client.Input(append([]byte(nil), frame...))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rtt, err := client.SendEcho(ctx)
	if err != nil {
		t.Fatalf("SendEcho: %v", err)
	}
	if rtt < 0 {
		t.Fatalf("negative RTT: %v", rtt)
	}

	// Test graceful Terminate.
	if err := client.Terminate(ctx); err != nil {
		t.Fatalf("Terminate: %v", err)
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
	// Drive LCP to Opened manually.
	_, lcpPayload, _ := DecodeFrame(lastFrame)
	reqPkt, _ := ParsePacket(lcpPayload)
	s.Input(EncodeFrame(ProtocolLCP, Packet{Code: CodeConfigureAck, ID: reqPkt.ID, Body: reqPkt.Body}.Marshal()))
	s.Input(EncodeFrame(ProtocolLCP, Packet{Code: CodeConfigureRequest, ID: 7, Body: nil}.Marshal()))

	// Now send an unsupported CCP (0x80FD) frame; client must reply with LCP Protocol-Reject.
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
	// Peer sends LCP Protocol-Reject for IPCP (0x8021)
	var body [2]byte
	binary.BigEndian.PutUint16(body[:], ProtocolIPCP)
	s.Input(EncodeFrame(ProtocolLCP, Packet{Code: CodeProtocolReject, ID: 1, Body: body[:]}.Marshal()))
	if closedErr == nil {
		t.Fatal("expected session to fail fast when peer sends Protocol-Reject for IPCP")
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
		}
	})
}
