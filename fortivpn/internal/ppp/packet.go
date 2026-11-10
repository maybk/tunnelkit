// Package ppp implements client-side RFC 1661 (PPP / LCP), RFC 1332 (IPCP),
// and RFC 1877 (IPCP DNS extensions) framing and negotiation for Fortinet
// SSL-VPN tunnels.
package ppp

import "encoding/binary"

// PPP protocol numbers (RFC 1661 / IANA PPP Assigned Numbers).
const (
	ProtocolIP     uint16 = 0x0021
	ProtocolIPv6   uint16 = 0x0057
	ProtocolIPCP   uint16 = 0x8021
	ProtocolIPv6CP uint16 = 0x8057
	ProtocolCCP    uint16 = 0x80fd
	ProtocolLCP    uint16 = 0xc021
)

// HeaderLen is the 4-byte HDLC-like PPP header length ([0xFF, 0x03, proto_hi, proto_lo]).
const (
	HeaderLen   = 4
	hdlcAddress = 0xff
	hdlcControl = 0x03
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

// PutFrameHeader writes the 4-octet PPP header [0xFF, 0x03, proto>>8, proto] into dst[:4].
func PutFrameHeader(dst []byte, protocol uint16) {
	_ = dst[3]
	dst[0] = hdlcAddress
	dst[1] = hdlcControl
	binary.BigEndian.PutUint16(dst[2:4], protocol)
}

// EncodeFrame prepends the 4-octet PPP header to payload in a newly allocated slice.
func EncodeFrame(protocol uint16, payload []byte) []byte {
	frame := make([]byte, 0, HeaderLen+len(payload))
	frame = append(frame, hdlcAddress, hdlcControl)
	frame = binary.BigEndian.AppendUint16(frame, protocol)
	return append(frame, payload...)
}

// DecodeFrame strips the PPP header (supporting optional 0xFF 0x03 ACFC and
// 1-octet or 2-octet PFC protocol fields) and returns the protocol number and payload slice.
func DecodeFrame(raw []byte) (protocol uint16, payload []byte, ok bool) {
	offset := 0
	if len(raw) >= 2 && raw[0] == hdlcAddress && raw[1] == hdlcControl {
		offset = 2
	}
	// The protocol's low octet is odd; PFC omits its zero high octet.
	for octets := 0; octets < 2 && offset < len(raw); octets++ {
		protocol = protocol<<8 | uint16(raw[offset])
		offset++
		if protocol&1 != 0 {
			return protocol, raw[offset:], true
		}
	}
	return 0, nil, false
}

// EncapsulateIP wraps an IPv4 packet in a 4-byte PPP data frame (protocol 0x0021).
func EncapsulateIP(ipPacket []byte) []byte {
	return EncodeFrame(ProtocolIP, ipPacket)
}

// ExtractIPv4 extracts an IPv4 packet from a PPP frame and trims any trailing
// link-layer padding using the IPv4 header's Total Length field.
func ExtractIPv4(frame []byte) ([]byte, bool) {
	proto, payload, ok := DecodeFrame(frame)
	if !ok || proto != ProtocolIP {
		return nil, false
	}
	return TrimToIPv4(payload)
}

// TrimToIPv4 validates an IPv4 packet header and returns a subslice bounded by Total Length.
func TrimToIPv4(pkt []byte) ([]byte, bool) {
	if len(pkt) < 20 || (pkt[0]>>4) != 4 {
		return nil, false
	}
	headerLen := int(pkt[0]&0x0f) * 4
	if headerLen < 20 || len(pkt) < headerLen {
		return nil, false
	}
	totalLen := int(binary.BigEndian.Uint16(pkt[2:4]))
	if totalLen < headerLen || totalLen > len(pkt) {
		return nil, false
	}
	return pkt[:totalLen], true
}

// Packet represents an LCP or IPCP control packet (Code, Identifier, 16-bit Length, Body).
type Packet struct {
	Code byte
	ID   byte
	Body []byte
}

const controlHeaderLen = 4

// ParsePacket decodes a control packet from raw and verifies its length header.
func ParsePacket(raw []byte) (Packet, bool) {
	if len(raw) >= controlHeaderLen {
		end := int(binary.BigEndian.Uint16(raw[2:controlHeaderLen]))
		if end >= controlHeaderLen && end <= len(raw) {
			return Packet{Code: raw[0], ID: raw[1], Body: raw[controlHeaderLen:end:end]}, true
		}
	}
	return Packet{}, false
}

// Marshal serializes the control packet with its 4-octet [Code, ID, Length] header.
func (p Packet) Marshal() []byte {
	encoded := make([]byte, 0, controlHeaderLen+len(p.Body))
	encoded = append(encoded, p.Code, p.ID)
	encoded = binary.BigEndian.AppendUint16(encoded, uint16(controlHeaderLen+len(p.Body)))
	return append(encoded, p.Body...)
}

// Option represents a single TLV configuration option inside a Configure-* packet.
type Option struct {
	Type  byte
	Value []byte
}

// OptionCursor iterates over TLV options in a Configure-* packet body without heap allocations.
type OptionCursor struct {
	rem     []byte
	corrupt bool
}

// NewOptionCursor returns a zero-allocation TLV cursor over body.
func NewOptionCursor(body []byte) OptionCursor {
	return OptionCursor{rem: body}
}

// Next advances the cursor and returns the next TLV option slice.
func (c *OptionCursor) Next() (Option, bool) {
	if len(c.rem) == 0 {
		return Option{}, false
	}
	if len(c.rem) < 2 {
		c.corrupt = true
		c.rem = nil
		return Option{}, false
	}
	optLen := int(c.rem[1])
	if optLen < 2 || optLen > len(c.rem) {
		c.corrupt = true
		c.rem = nil
		return Option{}, false
	}
	opt := Option{
		Type:  c.rem[0],
		Value: c.rem[2:optLen:optLen],
	}
	c.rem = c.rem[optLen:]
	return opt, true
}

// Valid reports whether the cursor reached the end of the buffer without encountering malformed TLVs.
func (c *OptionCursor) Valid() bool {
	return !c.corrupt && len(c.rem) == 0
}

// ParseOptions decodes TLV options from a Configure-* packet body without copying value bytes.
func ParseOptions(body []byte) ([]Option, bool) {
	if len(body) == 0 {
		return nil, true
	}
	count := 0
	cur := NewOptionCursor(body)
	for {
		if _, ok := cur.Next(); !ok {
			break
		}
		count++
	}
	if !cur.Valid() {
		return nil, false
	}
	opts := make([]Option, 0, count)
	cur = NewOptionCursor(body)
	for {
		opt, ok := cur.Next()
		if !ok {
			break
		}
		opts = append(opts, opt)
	}
	return opts, true
}

// MarshalOptions encodes a slice of TLV options into a single pre-sized buffer.
func MarshalOptions(opts []Option) []byte {
	total := 0
	for _, opt := range opts {
		total += 2 + len(opt.Value)
	}
	if total == 0 {
		return nil
	}
	buf := make([]byte, total)
	offset := 0
	for _, opt := range opts {
		optLen := 2 + len(opt.Value)
		buf[offset] = opt.Type
		buf[offset+1] = byte(optLen)
		copy(buf[offset+2:offset+optLen], opt.Value)
		offset += optLen
	}
	return buf
}
