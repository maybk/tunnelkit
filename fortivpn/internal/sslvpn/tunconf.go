package sslvpn

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// TunnelConfig holds the VPN parameters returned by /remote/fortisslvpn_xml
// (or legacy /remote/fortisslvpn).
type TunnelConfig struct {
	AssignedIP             netip.Addr
	DNS                    []netip.Addr
	DNSDomains             []string
	SplitRoutes            []netip.Prefix
	ExcludeRoutes          []netip.Prefix
	IdleTimeout            int
	AuthTimeout            int
	FortiOS                string
	ReconnectWithoutReauth bool
	RawXML                 []byte
}

type xmlTunnel struct {
	XMLName xml.Name `xml:"sslvpn-tunnel"`
	FOS     struct {
		Platform string `xml:"platform,attr"`
		Major    string `xml:"major,attr"`
		Minor    string `xml:"minor,attr"`
		Patch    string `xml:"patch,attr"`
	} `xml:"fos"`
	AuthSes struct {
		ConnectWithoutReauth string `xml:"tun-connect-without-reauth,attr"`
	} `xml:"auth-ses"`
	IPv4        xmlIPv4 `xml:"ipv4"`
	IdleTimeout struct {
		Val string `xml:"val,attr"`
	} `xml:"idle-timeout"`
	AuthTimeout struct {
		Val string `xml:"val,attr"`
	} `xml:"auth-timeout"`
	// Legacy flat FortiOS XML attributes/elements directly under <sslvpn-tunnel>:
	LegacyAssigned struct {
		IPv4 string `xml:"ipv4,attr"`
	} `xml:"assigned-addr"`
	LegacyDNS       []xmlDNSEntry      `xml:"dns"`
	LegacyDNSSuffix []xmlDNSSuffix     `xml:"dns-suffix"`
	LegacySplitDNS  []xmlSplitDNSEntry `xml:"split-dns"`
	LegacySplits    []xmlSplit         `xml:"split-tunnel-info"`
}

type xmlDNSEntry struct {
	IP     string `xml:"ip,attr"`
	Domain string `xml:"domain,attr"`
	Suffix string `xml:"suffix,attr"`
	Search string `xml:"search,attr"`
}

type xmlDNSSuffix struct {
	Val  string `xml:"val,attr"`
	Text string `xml:",chardata"`
}

type xmlSplitDNSEntry struct {
	Domains    string `xml:"domains,attr"`
	DNSServer1 string `xml:"dnsserver1,attr"`
	DNSServer2 string `xml:"dnsserver2,attr"`
}

type xmlIPv4 struct {
	Assigned struct {
		IPv4 string `xml:"ipv4,attr"`
	} `xml:"assigned-addr"`
	DNS       []xmlDNSEntry      `xml:"dns"`
	DNSSuffix []xmlDNSSuffix     `xml:"dns-suffix"`
	SplitDNS  []xmlSplitDNSEntry `xml:"split-dns"`
	Splits    []xmlSplit         `xml:"split-tunnel-info"`
}

type xmlSplit struct {
	Negate string `xml:"negate,attr"`
	Addrs  []struct {
		IP   string `xml:"ip,attr"`
		Mask string `xml:"mask,attr"`
	} `xml:"addr"`
}

// ParseConfigXML decodes a /remote/fortisslvpn_xml document, deduplicating
// DNS servers, domains, and split-tunnel routes and discarding 0.0.0.0 entries.
func ParseConfigXML(data []byte) (TunnelConfig, error) {
	var t xmlTunnel
	if err := xml.Unmarshal(data, &t); err != nil {
		return TunnelConfig{}, fmt.Errorf("sslvpn: parsing config XML: %w", err)
	}

	cfg := TunnelConfig{
		RawXML:                 append([]byte(nil), data...),
		ReconnectWithoutReauth: t.AuthSes.ConnectWithoutReauth == "1",
	}

	if t.FOS.Platform != "" || t.FOS.Major != "" {
		ver := strings.Trim(strings.Join([]string{t.FOS.Major, t.FOS.Minor, t.FOS.Patch}, "."), ".")
		if t.FOS.Platform != "" && ver != "" {
			cfg.FortiOS = t.FOS.Platform + " v" + ver
		} else if t.FOS.Platform != "" {
			cfg.FortiOS = t.FOS.Platform
		} else {
			cfg.FortiOS = "v" + ver
		}
	}

	if v, err := strconv.Atoi(t.IdleTimeout.Val); err == nil && v > 0 {
		cfg.IdleTimeout = v
	}
	if v, err := strconv.Atoi(t.AuthTimeout.Val); err == nil && v > 0 {
		cfg.AuthTimeout = v
	}

	assignedRaw := strings.TrimSpace(t.IPv4.Assigned.IPv4)
	if assignedRaw == "" {
		assignedRaw = strings.TrimSpace(t.LegacyAssigned.IPv4)
	}
	if assignedRaw != "" {
		addr, err := netip.ParseAddr(assignedRaw)
		if err != nil || !addr.Is4() {
			return TunnelConfig{}, fmt.Errorf("sslvpn: invalid assigned-addr %q", assignedRaw)
		}
		cfg.AssignedIP = addr
	}

	dnsSeen := map[netip.Addr]struct{}{}
	addDNS := func(raw string) {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil || !addr.Is4() || addr.IsUnspecified() {
			return
		}
		if _, ok := dnsSeen[addr]; ok {
			return
		}
		dnsSeen[addr] = struct{}{}
		cfg.DNS = append(cfg.DNS, addr)
	}

	domainSeen := map[string]struct{}{}
	addDomain := func(raw string) {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}) {
			d := strings.ToLower(strings.Trim(strings.TrimSpace(part), "."))
			if d == "" {
				continue
			}
			// Reject misconfigured IP address literals in DNS domain attributes.
			if _, err := netip.ParseAddr(d); err == nil {
				continue
			}
			if _, ok := domainSeen[d]; ok {
				continue
			}
			domainSeen[d] = struct{}{}
			cfg.DNSDomains = append(cfg.DNSDomains, d)
		}
	}

	for _, d := range append(append([]xmlDNSEntry(nil), t.IPv4.DNS...), t.LegacyDNS...) {
		addDNS(d.IP)
		addDomain(d.Domain)
		addDomain(d.Suffix)
		addDomain(d.Search)
	}
	for _, sfx := range append(append([]xmlDNSSuffix(nil), t.IPv4.DNSSuffix...), t.LegacyDNSSuffix...) {
		addDomain(sfx.Val)
		addDomain(sfx.Text)
	}
	for _, sd := range append(append([]xmlSplitDNSEntry(nil), t.IPv4.SplitDNS...), t.LegacySplitDNS...) {
		addDomain(sd.Domains)
		addDNS(sd.DNSServer1)
		addDNS(sd.DNSServer2)
	}

	incSeen := map[netip.Prefix]struct{}{}
	excSeen := map[netip.Prefix]struct{}{}
	for _, sp := range append(append([]xmlSplit(nil), t.IPv4.Splits...), t.LegacySplits...) {
		for _, a := range sp.Addrs {
			pfx, ok := ipMaskToPrefix(a.IP, a.Mask)
			if !ok {
				continue
			}
			if sp.Negate == "1" {
				if _, exists := excSeen[pfx]; !exists {
					excSeen[pfx] = struct{}{}
					cfg.ExcludeRoutes = append(cfg.ExcludeRoutes, pfx)
				}
			} else {
				if _, exists := incSeen[pfx]; !exists {
					incSeen[pfx] = struct{}{}
					cfg.SplitRoutes = append(cfg.SplitRoutes, pfx)
				}
			}
		}
	}

	return cfg, nil
}

// BuildConfigXML renders a TunnelConfig into a fortisslvpn_xml document (for fortitest.Gateway).
func BuildConfigXML(cfg TunnelConfig) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<sslvpn-tunnel ver="2" dtls="0">`)
	if cfg.FortiOS != "" {
		fmt.Fprintf(&b, `<fos platform=%q major="7" minor="4" patch="1"/>`, cfg.FortiOS)
	}
	if cfg.ReconnectWithoutReauth {
		b.WriteString(`<auth-ses tun-connect-without-reauth="1"/>`)
	}
	b.WriteString(`<ipv4>`)
	if cfg.AssignedIP.IsValid() {
		fmt.Fprintf(&b, `<assigned-addr ipv4=%q/>`, cfg.AssignedIP.String())
	}
	for i, d := range cfg.DNS {
		if i == 0 && len(cfg.DNSDomains) > 0 {
			fmt.Fprintf(&b, `<dns ip=%q domain=%q/>`, d.String(), cfg.DNSDomains[0])
		} else {
			fmt.Fprintf(&b, `<dns ip=%q/>`, d.String())
		}
	}
	if len(cfg.DNSDomains) > 1 {
		fmt.Fprintf(&b, `<split-dns domains=%q/>`, strings.Join(cfg.DNSDomains, ","))
	}
	writeXMLRoutes(&b, cfg.SplitRoutes, false)
	writeXMLRoutes(&b, cfg.ExcludeRoutes, true)
	b.WriteString(`</ipv4>`)
	idle := cfg.IdleTimeout
	if idle <= 0 {
		idle = 3600
	}
	auth := cfg.AuthTimeout
	if auth <= 0 {
		auth = 18000
	}
	fmt.Fprintf(&b, `<idle-timeout val="%d"/><auth-timeout val="%d"/>`, idle, auth)
	b.WriteString(`</sslvpn-tunnel>`)
	return []byte(b.String())
}

func writeXMLRoutes(b *strings.Builder, routes []netip.Prefix, negate bool) {
	if len(routes) == 0 {
		return
	}
	if negate {
		b.WriteString(`<split-tunnel-info negate="1">`)
	} else {
		b.WriteString(`<split-tunnel-info>`)
	}
	for _, pfx := range routes {
		masked := pfx.Masked()
		mask := net.CIDRMask(masked.Bits(), 32)
		fmt.Fprintf(b, `<addr ip=%q mask=%q/>`, masked.Addr().String(), net.IP(mask).String())
	}
	b.WriteString(`</split-tunnel-info>`)
}

var legacyIPRegex = regexp.MustCompile(`(\d{1,3}(?:\.\d{1,3}){3})/(\d{1,3}(?:\.\d{1,3}){3})`)

// ParseLegacyConfigHTML extracts route pairs from the legacy /remote/fortisslvpn response.
func ParseLegacyConfigHTML(data []byte) (TunnelConfig, error) {
	var cfg TunnelConfig
	matches := legacyIPRegex.FindAllStringSubmatch(string(data), -1)
	seen := map[netip.Prefix]struct{}{}
	for _, m := range matches {
		if pfx, ok := ipMaskToPrefix(m[1], m[2]); ok {
			if _, exists := seen[pfx]; !exists {
				seen[pfx] = struct{}{}
				cfg.SplitRoutes = append(cfg.SplitRoutes, pfx)
			}
		}
	}
	return cfg, nil
}

var redactAttrRegex = regexp.MustCompile(`(?i)\b(username|user|group|cookie|token|password|passwd)="[^"]*"`)

// RedactConfigXML masks cookie or user tokens in raw XML before writing --dump-config.
func RedactConfigXML(data []byte) []byte {
	s := string(data)
	s = strings.ReplaceAll(s, CookieName, "REDACTED_COOKIE")
	s = redactAttrRegex.ReplaceAllString(s, `$1="REDACTED"`)
	return []byte(s)
}

func ipMaskToPrefix(ipStr, maskStr string) (netip.Prefix, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.Is4() {
		return netip.Prefix{}, false
	}
	maskIP := net.ParseIP(strings.TrimSpace(maskStr)).To4()
	if maskIP == nil {
		return netip.Prefix{}, false
	}
	ones, bits := net.IPMask(maskIP).Size()
	if bits != 32 || ones < 0 {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, ones).Masked(), true
}
