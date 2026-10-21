// Package sslvpn implements the FortiOS SSL-VPN HTTPS control plane, certificate
// trust verification, tunnel upgrade, and 6-byte 0x5050 PPP frame transport.
package sslvpn

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	// FrameHeaderLen is the 6-octet Fortinet framing header size:
	// [2B total length BE][2B 0x5050 magic][2B payload length BE].
	FrameHeaderLen = 6
	// FrameMagic is the constant 16-bit magic value (ASCII "PP") at bytes 2..3.
	FrameMagic = 0x5050
	// MaxFramePayload bounds a single PPP frame payload (64 KiB - 1).
	MaxFramePayload = (1 << 16) - 1 - FrameHeaderLen
)

var (
	ErrShortFrame     = errors.New("sslvpn: record shorter than its header")
	ErrBadMagic       = errors.New("sslvpn: record magic is not 0x5050")
	ErrLengthMismatch = errors.New("sslvpn: record length fields disagree")
	ErrFrameTooLarge  = errors.New("sslvpn: PPP frame exceeds the maximum")
)

// EncodeFrame wraps a PPP frame in the 6-octet Fortinet header.
func EncodeFrame(pppFrame []byte) []byte {
	out := make([]byte, FrameHeaderLen+len(pppFrame))
	EncodeFrameInto(out, pppFrame)
	return out
}

// EncodeFrameInto writes the 6-octet Fortinet header and pppFrame into dst,
// which must have length >= FrameHeaderLen + len(pppFrame).
func EncodeFrameInto(dst, pppFrame []byte) int {
	total := FrameHeaderLen + len(pppFrame)
	binary.BigEndian.PutUint16(dst[0:2], uint16(total))
	binary.BigEndian.PutUint16(dst[2:4], FrameMagic)
	binary.BigEndian.PutUint16(dst[4:6], uint16(len(pppFrame)))
	copy(dst[FrameHeaderLen:], pppFrame)
	return total
}

// ParseFrame parses the 6-octet header at the start of buf and returns the
// PPP frame slice and the remaining unconsumed bytes.
func ParseFrame(buf []byte) (pppFrame, rest []byte, err error) {
	if len(buf) < FrameHeaderLen {
		return nil, buf, ErrShortFrame
	}
	total := int(binary.BigEndian.Uint16(buf[0:2]))
	magic := binary.BigEndian.Uint16(buf[2:4])
	plen := int(binary.BigEndian.Uint16(buf[4:6]))

	if magic != FrameMagic {
		return nil, buf, ErrBadMagic
	}
	if total != plen+FrameHeaderLen {
		return nil, buf, ErrLengthMismatch
	}
	if plen > MaxFramePayload {
		return nil, buf, ErrFrameTooLarge
	}
	if len(buf) < FrameHeaderLen+plen {
		return nil, buf, ErrShortFrame
	}
	return buf[FrameHeaderLen : FrameHeaderLen+plen], buf[FrameHeaderLen+plen:], nil
}

// ReadFrame reads a single framed PPP record from r.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [FrameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	total := int(binary.BigEndian.Uint16(hdr[0:2]))
	magic := binary.BigEndian.Uint16(hdr[2:4])
	plen := int(binary.BigEndian.Uint16(hdr[4:6]))

	if magic != FrameMagic {
		return nil, ErrBadMagic
	}
	if total != plen+FrameHeaderLen {
		return nil, ErrLengthMismatch
	}
	if plen > MaxFramePayload {
		return nil, ErrFrameTooLarge
	}
	pppFrame := make([]byte, plen)
	if _, err := io.ReadFull(r, pppFrame); err != nil {
		return nil, err
	}
	return pppFrame, nil
}

// ReadFrameInto reads a single framed PPP record from r using scratch as the
// destination buffer if its capacity is sufficient.
func ReadFrameInto(r io.Reader, scratch []byte) ([]byte, error) {
	var hdr [FrameHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	total := int(binary.BigEndian.Uint16(hdr[0:2]))
	magic := binary.BigEndian.Uint16(hdr[2:4])
	plen := int(binary.BigEndian.Uint16(hdr[4:6]))

	if magic != FrameMagic {
		return nil, ErrBadMagic
	}
	if total != plen+FrameHeaderLen {
		return nil, ErrLengthMismatch
	}
	if plen > MaxFramePayload {
		return nil, ErrFrameTooLarge
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
