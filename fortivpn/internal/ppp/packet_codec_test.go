package ppp

import (
	"bytes"
	"testing"
)

func TestPPPWireFormats(t *testing.T) {
	payload := []byte{0x45, 0, 0, 0x14}
	for _, header := range [][]byte{{0xff, 3, 0, 0x21}, {0, 0x21}, {0xff, 3, 0x21}, {0x21}} {
		raw := append(append([]byte(nil), header...), payload...)
		protocol, body, ok := DecodeFrame(raw)
		if !ok || protocol != ProtocolIP || !bytes.Equal(body, payload) {
			t.Fatalf("could not decode header %x", header)
		}
	}
	want := []byte{0xff, 3, 0xc0, 0x21, 1, 7, 0, 4}
	if got := EncodeFrame(ProtocolLCP, []byte{1, 7, 0, 4}); !bytes.Equal(got, want) {
		t.Fatalf("LCP frame: %x", got)
	}
}

func TestControlPacketWireLengthExcludesPadding(t *testing.T) {
	wire := []byte{1, 7, 0, 8, 1, 4, 5, 0x78}
	packet, ok := ParsePacket(append(append([]byte(nil), wire...), 0, 0))
	if !ok || packet.Code != CodeConfigureRequest || packet.ID != 7 || !bytes.Equal(packet.Body, wire[4:]) {
		t.Fatal("control packet length includes padding")
	}
	if encoded := packet.Marshal(); !bytes.Equal(encoded, wire) {
		t.Fatalf("control packet wire format: %x", encoded)
	}
}
