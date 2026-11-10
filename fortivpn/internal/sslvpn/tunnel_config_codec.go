package sslvpn

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// ParseConfigXML parses a /remote/fortisslvpn_xml document using a streaming XML token
// decoder, deduplicating DNS servers, search domains, and split/exclude routes across
// both modern (<ipv4>) and legacy flat FortiOS XML schemas.
func ParseConfigXML(data []byte) (TunnelConfig, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	cfg := TunnelConfig{
		RawXML: append([]byte(nil), data...),
	}

	dnsSeen := make(map[netip.Addr]struct{})
	addDNS := func(raw string) {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil || !addr.Is4() || addr.IsUnspecified() {
			return
		}
		if _, ok := dnsSeen[addr]; !ok {
			dnsSeen[addr] = struct{}{}
			cfg.DNS = append(cfg.DNS, addr)
		}
	}

	domainSeen := make(map[string]struct{})
	addDomain := func(raw string) {
		for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}) {
			d := strings.ToLower(strings.Trim(strings.TrimSpace(token), "."))
			if d == "" {
				continue
			}
			// Ignore IP literals mistakenly placed in domain attributes by gateway admins.
			if _, err := netip.ParseAddr(d); err == nil {
				continue
			}
			if _, ok := domainSeen[d]; !ok {
				domainSeen[d] = struct{}{}
				cfg.DNSDomains = append(cfg.DNSDomains, d)
			}
		}
	}

	incSeen := make(map[netip.Prefix]struct{})
	excSeen := make(map[netip.Prefix]struct{})
	addRoute := func(pfx netip.Prefix, negate bool) {
		pfx = pfx.Masked()
		if negate {
			if _, ok := excSeen[pfx]; !ok {
				excSeen[pfx] = struct{}{}
				cfg.ExcludeRoutes = append(cfg.ExcludeRoutes, pfx)
			}
		} else {
			if _, ok := incSeen[pfx]; !ok {
				incSeen[pfx] = struct{}{}
				cfg.SplitRoutes = append(cfg.SplitRoutes, pfx)
			}
		}
	}

	type elemState struct {
		name   string
		negate bool
	}
	var (
		stack          []elemState
		sawRoot        bool
		assignedIPv4   string
		fallbackIPv4   string
		pendingSuffixs []string
	)

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return TunnelConfig{}, fmt.Errorf("sslvpn: parsing config XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			attrs := make(map[string]string, len(t.Attr))
			for _, a := range t.Attr {
				attrs[strings.ToLower(a.Name.Local)] = strings.TrimSpace(a.Value)
			}

			inNegateSplit := false
			inIPv4 := false
			for _, s := range stack {
				if s.name == "split-tunnel-info" && s.negate {
					inNegateSplit = true
				}
				if s.name == "ipv4" {
					inIPv4 = true
				}
			}
			if name == "split-tunnel-info" && attrs["negate"] == "1" {
				inNegateSplit = true
			}
			if name == "sslvpn-tunnel" {
				sawRoot = true
			}
			stack = append(stack, elemState{name: name, negate: inNegateSplit})

			switch name {
			case "fos":
				platform := attrs["platform"]
				ver := strings.Trim(strings.Join([]string{attrs["major"], attrs["minor"], attrs["patch"]}, "."), ".")
				if platform != "" && ver != "" {
					cfg.FortiOS = platform + " v" + ver
				} else if platform != "" {
					cfg.FortiOS = platform
				} else if ver != "" {
					cfg.FortiOS = "v" + ver
				}
			case "auth-ses":
				if attrs["tun-connect-without-reauth"] == "1" {
					cfg.ReconnectWithoutReauth = true
				}
			case "idle-timeout":
				if v, err := strconv.Atoi(attrs["val"]); err == nil && v > 0 {
					cfg.IdleTimeout = v
				}
			case "auth-timeout":
				if v, err := strconv.Atoi(attrs["val"]); err == nil && v > 0 {
					cfg.AuthTimeout = v
				}
			case "assigned-addr", "assigned_addr":
				ipVal := firstNonEmpty(attrs["ipv4"], attrs["ip"], attrs["addr"])
				if ipVal != "" {
					if inIPv4 {
						assignedIPv4 = ipVal
					} else if fallbackIPv4 == "" {
						fallbackIPv4 = ipVal
					}
				}
			case "dns", "dns-server", "dns_server":
				addDNS(firstNonEmpty(attrs["ip"], attrs["ipv4"], attrs["addr"], attrs["address"]))
				addDomain(attrs["domain"])
				addDomain(attrs["suffix"])
				addDomain(attrs["search"])
			case "dns-suffix", "dns_suffix":
				if val := attrs["val"]; val != "" {
					if inIPv4 {
						addDomain(val)
					} else {
						pendingSuffixs = append(pendingSuffixs, val)
					}
				}
			case "split-dns", "split_dns":
				addDomain(attrs["domains"])
				for i := 1; i <= 9; i++ {
					addDNS(attrs["dnsserver"+strconv.Itoa(i)])
				}
			case "addr", "route", "routev4":
				ipStr := firstNonEmpty(attrs["ip"], attrs["dest"], attrs["destination"], attrs["network"], attrs["addr"], attrs["ipv4"])
				maskStr := firstNonEmpty(attrs["mask"], attrs["netmask"])
				if pfx, ok := ipMaskToPrefix(ipStr, maskStr); ok {
					addRoute(pfx, inNegateSplit)
				}
			}

		case xml.CharData:
			if len(stack) > 0 {
				top := stack[len(stack)-1].name
				if top == "dns-suffix" || top == "dns_suffix" {
					addDomain(string(t))
				}
			}

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if !sawRoot {
		return TunnelConfig{}, fmt.Errorf("sslvpn: missing <sslvpn-tunnel> root element")
	}

	for _, sfx := range pendingSuffixs {
		addDomain(sfx)
	}

	chosenIP := assignedIPv4
	if chosenIP == "" {
		chosenIP = fallbackIPv4
	}
	if chosenIP != "" {
		addr, err := netip.ParseAddr(chosenIP)
		if err != nil || !addr.Is4() {
			return TunnelConfig{}, fmt.Errorf("sslvpn: invalid assigned-addr %q", chosenIP)
		}
		cfg.AssignedIP = addr
	}

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var legacyIPRegex = regexp.MustCompile(`(\d{1,3}(?:\.\d{1,3}){3})/(\d{1,3}(?:\.\d{1,3}){3})`)

// ParseLegacyConfigHTML extracts route pairs from the legacy /remote/fortisslvpn response.
func ParseLegacyConfigHTML(data []byte) (TunnelConfig, error) {
	var cfg TunnelConfig
	matches := legacyIPRegex.FindAllStringSubmatch(string(data), -1)
	seen := make(map[netip.Prefix]struct{})
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

func ipMaskToPrefix(ipStr, maskStr string) (netip.Prefix, bool) {
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" {
		return netip.Prefix{}, false
	}
	if pfx, err := netip.ParsePrefix(ipStr); err == nil && pfx.Addr().Is4() {
		return pfx.Masked(), true
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil || !addr.Is4() {
		return netip.Prefix{}, false
	}
	maskStr = strings.TrimSpace(maskStr)
	if maskStr == "" {
		return netip.PrefixFrom(addr, 32), true
	}
	if bits, err := strconv.Atoi(strings.TrimPrefix(maskStr, "/")); err == nil && bits >= 0 && bits <= 32 {
		return netip.PrefixFrom(addr, bits).Masked(), true
	}
	maskIP := net.ParseIP(maskStr).To4()
	if maskIP == nil {
		return netip.Prefix{}, false
	}
	ones, bits := net.IPMask(maskIP).Size()
	if bits != 32 || ones < 0 {
		return netip.Prefix{}, false
	}
	return netip.PrefixFrom(addr, ones).Masked(), true
}
