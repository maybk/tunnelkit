// Package sslvpn implements the client-side FortiOS SSL-VPN HTTPS control plane,
// TLS certificate pinning, tunnel upgrade, and 6-byte 0x5050 PPP frame transport.
package sslvpn

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"tunnelkit/fortivpn"
)

const (
	// FrameHeaderLen is the 6-octet Fortinet tunnel header size:
	// [2B total length BE][2B 0x5050 magic][2B PPP payload length BE].
	FrameHeaderLen = 6
	// FrameMagic is the 16-bit Fortinet tunnel magic value ("PP" = 0x5050).
	FrameMagic uint16 = 0x5050
	// MaxFramePayload is the maximum PPP frame size that fits in a 16-bit total length field.
	MaxFramePayload = 0xffff - FrameHeaderLen
)

var (
	ErrShortFrame     = errors.New("sslvpn: frame buffer shorter than declared length")
	ErrBadMagic       = errors.New("sslvpn: invalid frame magic (expected 0x5050)")
	ErrLengthMismatch = errors.New("sslvpn: frame total length does not match payload length")
	ErrFrameTooLarge  = errors.New("sslvpn: frame payload exceeds maximum size")
)

// PutFrameHeader writes the 6-octet Fortinet frame header for a PPP payload of length payloadLen into dst[:6].
func PutFrameHeader(dst []byte, payloadLen int) {
	_ = dst[5]
	total := FrameHeaderLen + payloadLen
	binary.BigEndian.PutUint16(dst[0:2], uint16(total))
	binary.BigEndian.PutUint16(dst[2:4], FrameMagic)
	binary.BigEndian.PutUint16(dst[4:6], uint16(payloadLen))
}

// EncodeFrameInto writes the 6-octet Fortinet header and copies pppFrame into dst.
func EncodeFrameInto(dst, pppFrame []byte) int {
	PutFrameHeader(dst, len(pppFrame))
	copy(dst[FrameHeaderLen:], pppFrame)
	return FrameHeaderLen + len(pppFrame)
}

// EncodeFrame allocates and returns a new Fortinet-framed buffer wrapping pppFrame.
func EncodeFrame(pppFrame []byte) []byte {
	out := make([]byte, FrameHeaderLen+len(pppFrame))
	EncodeFrameInto(out, pppFrame)
	return out
}

func validateHeader(hdr []byte) (payloadLen int, err error) {
	if len(hdr) < FrameHeaderLen {
		return 0, ErrShortFrame
	}
	total := int(binary.BigEndian.Uint16(hdr[0:2]))
	magic := binary.BigEndian.Uint16(hdr[2:4])
	plen := int(binary.BigEndian.Uint16(hdr[4:6]))
	if magic != FrameMagic {
		trimmed := bytes.TrimLeft(hdr, " \t\r\n")
		if len(trimmed) > 0 && trimmed[0] == '<' {
			return 0, fortivpn.NewError(fortivpn.CodeTunnelDenied, "gateway returned HTML page instead of binary tunnel (tunnel mode disabled)")
		}
		return 0, ErrBadMagic
	}
	if total != FrameHeaderLen+plen {
		return 0, ErrLengthMismatch
	}
	if plen > MaxFramePayload {
		return 0, ErrFrameTooLarge
	}
	return plen, nil
}

// ParseFrame decodes one Fortinet-framed PPP record from the start of buf and
// returns the PPP payload slice along with the remaining unconsumed bytes.
func ParseFrame(buf []byte) (pppFrame, rest []byte, err error) {
	plen, err := validateHeader(buf)
	if err != nil {
		return nil, buf, err
	}
	end := FrameHeaderLen + plen
	if len(buf) < end {
		return nil, buf, ErrShortFrame
	}
	return buf[FrameHeaderLen:end], buf[end:], nil
}

// ReadFrame reads a single Fortinet-framed PPP record from r into a newly allocated slice.
func ReadFrame(r io.Reader) ([]byte, error) {
	return ReadFrameInto(r, nil)
}

// ReadFrameInto reads a single Fortinet-framed PPP record from r, reusing scratch when cap(scratch) >= plen.
func ReadFrameInto(r io.Reader, scratch []byte) ([]byte, error) {
	var hdr []byte
	if cap(scratch) >= FrameHeaderLen {
		hdr = scratch[:FrameHeaderLen]
	} else {
		hdr = make([]byte, FrameHeaderLen)
	}
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	plen, err := validateHeader(hdr)
	if err != nil {
		return nil, err
	}
	var dst []byte
	if cap(scratch) >= plen {
		dst = scratch[:plen]
	} else {
		dst = make([]byte, plen)
	}
	if _, err := io.ReadFull(r, dst); err != nil {
		return nil, err
	}
	return dst, nil
}
