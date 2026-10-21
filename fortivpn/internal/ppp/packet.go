// Package ppp implements RFC 1661 (PPP / LCP), RFC 1332 (IPCP), and RFC 1877
// (IPCP DNS extensions) for Fortinet SSL-VPN PPP-over-TLS tunnels.
package ppp

import "encoding/binary"

// PPP protocol numbers.
const (
	ProtocolIP     uint16 = 0x0021
	ProtocolIPv6   uint16 = 0x0057
	ProtocolIPCP   uint16 = 0x8021
	ProtocolIPv6CP uint16 = 0x8057
	ProtocolCCP    uint16 = 0x80fd
	ProtocolLCP    uint16 = 0xc021
)

// Standard PPP HDLC address and control octets used inside Fortinet frames.
const (
	hdlcAddress    = 0xff
	hdlcControl    = 0x03
	frameHeaderLen = 4
)

// Control-protocol packet codes (RFC 1661 §5).
const (
	CodeConfigureRequest byte = 1
	CodeConfigureAck     byte = 2
	CodeConfigureNak     byte = 3
	CodeConfigureReject  byte = 4
	CodeTerminateRequest byte = 5
	CodeTerminateAck     byte = 6
	CodeCodeReject       byte = 7
	CodeProtocolReject   byte = 8
	CodeEchoRequest      byte = 9
	CodeEchoReply        byte = 10
	CodeDiscardRequest   byte = 11
)

// PutFrameHeader writes [0xFF, 0x03, proto>>8, proto] into the first 4 bytes of b.
func PutFrameHeader(b []byte, protocol uint16) {
	b[0] = hdlcAddress
	b[1] = hdlcControl
	b[2] = byte(protocol >> 8)
	b[3] = byte(protocol)
}

// EncodeFrame builds a 4-byte PPP header followed by payload.
func EncodeFrame(protocol uint16, payload []byte) []byte {
	out := make([]byte, frameHeaderLen+len(payload))
	PutFrameHeader(out, protocol)
	copy(out[frameHeaderLen:], payload)
	return out
}

// DecodeFrame parses a PPP frame into its protocol number and payload,
// tolerating an optional leading 0xFF 0x03 address/control pair and a
// compressed 1-byte protocol field.
func DecodeFrame(frame []byte) (protocol uint16, payload []byte, ok bool) {
	b := frame
	if len(b) >= 2 && b[0] == hdlcAddress && b[1] == hdlcControl {
		b = b[2:]
	}
	if len(b) == 0 {
		return 0, nil, false
	}
	if b[0]&1 == 1 {
		return uint16(b[0]), b[1:], true
	}
	if len(b) < 2 {
		return 0, nil, false
	}
	return uint16(b[0])<<8 | uint16(b[1]), b[2:], true
}

// EncapsulateIP wraps an IPv4 packet in a PPP frame (0xFF 0x03 0x00 0x21 + pkt).
func EncapsulateIP(ipPacket []byte) []byte {
	return EncodeFrame(ProtocolIP, ipPacket)
}

// ExtractIPv4 checks whether frame is a PPP IPv4 data frame and trims any
// trailing PPP padding using the IPv4 Total Length field (RFC 1661 §5.1).
func ExtractIPv4(frame []byte) ([]byte, bool) {
	proto, payload, ok := DecodeFrame(frame)
	if !ok || proto != ProtocolIP {
		return nil, false
	}
	return TrimToIPv4(payload)
}

// TrimToIPv4 validates an IPv4 packet header and slices it to its declared Total Length.
func TrimToIPv4(pkt []byte) ([]byte, bool) {
	if len(pkt) < 20 {
		return nil, false
	}
	version := pkt[0] >> 4
	ihl := int(pkt[0]&0x0f) * 4
	if version != 4 || ihl < 20 || len(pkt) < ihl {
		return nil, false
	}
	totalLen := int(binary.BigEndian.Uint16(pkt[2:4]))
	if totalLen < ihl || totalLen > len(pkt) {
		return nil, false
	}
	return pkt[:totalLen], true
}

// Packet is a PPP control-protocol packet (LCP or IPCP).
type Packet struct {
	Code byte
	ID   byte
	Body []byte
}

// ParsePacket decodes a control-protocol packet and validates its 16-bit length field.
func ParsePacket(b []byte) (Packet, bool) {
	if len(b) < 4 {
		return Packet{}, false
	}
	length := int(binary.BigEndian.Uint16(b[2:4]))
	if length < 4 || length > len(b) {
		return Packet{}, false
	}
	return Packet{
		Code: b[0],
		ID:   b[1],
		Body: b[4:length],
	}, true
}

// Marshal serializes a control-protocol packet with its 4-byte header.
func (p Packet) Marshal() []byte {
	out := make([]byte, 4+len(p.Body))
	out[0] = p.Code
	out[1] = p.ID
	binary.BigEndian.PutUint16(out[2:4], uint16(len(out)))
	copy(out[4:], p.Body)
	return out
}

// Option represents a single TLV configuration option in a Configure-* packet.
type Option struct {
	Type  byte
	Value []byte
}

// ParseOptions decodes a sequence of TLV options from a Configure-* packet body.
func ParseOptions(b []byte) ([]Option, bool) {
	var opts []Option
	for len(b) > 0 {
		if len(b) < 2 {
			return nil, false
		}
		length := int(b[1])
		if length < 2 || length > len(b) {
			return nil, false
		}
		val := make([]byte, length-2)
		copy(val, b[2:length])
		opts = append(opts, Option{Type: b[0], Value: val})
		b = b[length:]
	}
	return opts, true
}

// MarshalOptions encodes a slice of TLV options.
func MarshalOptions(opts []Option) []byte {
	var out []byte
	for _, o := range opts {
		out = append(out, o.Type, byte(2+len(o.Value)))
		out = append(out, o.Value...)
	}
	return out
}
