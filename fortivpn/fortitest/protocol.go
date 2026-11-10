package fortitest

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

func (g *Gateway) renderConfigXML(routes []netip.Prefix) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><sslvpn-tunnel ver="2" dtls="0">`)
	if g.cfg.FortiOS != "" {
		fmt.Fprintf(&b, `<fos platform=%q major="7" minor="4" patch="1"/>`, g.cfg.FortiOS)
	}
	if g.cfg.ReconnectWithoutReauth {
		b.WriteString(`<auth-ses tun-connect-without-reauth="1"/>`)
	}
	b.WriteString(`<ipv4>`)
	if g.cfg.AssignedIP.IsValid() {
		fmt.Fprintf(&b, `<assigned-addr ipv4=%q/>`, g.cfg.AssignedIP.String())
	}
	for i, d := range g.cfg.DNS {
		if i == 0 && len(g.cfg.DNSDomains) > 0 {
			fmt.Fprintf(&b, `<dns ip=%q domain=%q/>`, d.String(), g.cfg.DNSDomains[0])
		} else {
			fmt.Fprintf(&b, `<dns ip=%q/>`, d.String())
		}
	}
	if len(g.cfg.DNSDomains) > 1 {
		fmt.Fprintf(&b, `<split-dns domains=%q/>`, strings.Join(g.cfg.DNSDomains, ","))
	}
	if len(routes) > 0 {
		b.WriteString(`<split-tunnel-info>`)
		for _, pfx := range routes {
			masked := pfx.Masked()
			mask := net.CIDRMask(masked.Bits(), 32)
			fmt.Fprintf(&b, `<addr ip=%q mask=%q/>`, masked.Addr().String(), net.IP(mask).String())
		}
		b.WriteString(`</split-tunnel-info>`)
	}
	b.WriteString(`</ipv4><idle-timeout val="3600"/><auth-timeout val="18000"/></sslvpn-tunnel>`)
	return []byte(b.String())
}

func reflectIPv4(pkt []byte) []byte {
	out := make([]byte, len(pkt))
	copy(out, pkt)
	ihl := int(out[0]&0x0f) * 4
	if len(out) < 20 || ihl < 20 || len(out) < ihl {
		return out
	}
	for i := 0; i < 4; i++ {
		out[12+i], out[16+i] = out[16+i], out[12+i]
	}
	out[10], out[11] = 0, 0
	csum := ipv4Checksum(out[:ihl])
	binary.BigEndian.PutUint16(out[10:12], csum)

	if out[9] == 1 && len(out) >= ihl+8 && out[ihl] == 8 {
		out[ihl] = 0
		out[ihl+2], out[ihl+3] = 0, 0
		icmpSum := ipv4Checksum(out[ihl:])
		binary.BigEndian.PutUint16(out[ihl+2:ihl+4], icmpSum)
	}
	return out
}

func ipv4Checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i : i+2]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
