package ppp

import (
	"fmt"
	"net/netip"
)

// IPCP option types (RFC 1332 & RFC 1877).
const (
	OptIPAddress     byte = 3
	OptPrimaryDNS    byte = 0x81 // 129
	OptPrimaryNBNS   byte = 0x82 // 130
	OptSecondaryDNS  byte = 0x83 // 131
	OptSecondaryNBNS byte = 0x84 // 132
)

// IPConfig holds the IPv4 addresses and DNS servers negotiated over IPCP.
type IPConfig struct {
	LocalIP netip.Addr
	PeerIP  netip.Addr
	DNS     []netip.Addr
}

// IPCPHandler implements OptionHandler for the IP Control Protocol (0x8021).
type IPCPHandler struct {
	reqIP      [4]byte
	reqDNS1    [4]byte
	reqDNS2    [4]byte
	useDNS1    bool
	useDNS2    bool
	peerIP     netip.Addr
	fallbackIP netip.Addr
}

// NewIPCPHandler creates an IPCP option handler.
func NewIPCPHandler() *IPCPHandler {
	h := &IPCPHandler{}
	h.ResetOptions()
	return h
}

// SetFallbackIP sets the fallback local IPv4 address (e.g. from fortisslvpn_xml)
// used when the peer completes IPCP without assigning a non-zero IP.
func (h *IPCPHandler) SetFallbackIP(ip netip.Addr) {
	if ip.IsValid() && ip.Is4() && !ip.IsUnspecified() {
		h.fallbackIP = ip
	}
}

func (h *IPCPHandler) Protocol() uint16 { return ProtocolIPCP }
func (h *IPCPHandler) Name() string     { return "IPCP" }

func (h *IPCPHandler) ResetOptions() {
	h.reqIP = [4]byte{0, 0, 0, 0}
	h.reqDNS1 = [4]byte{0, 0, 0, 0}
	h.reqDNS2 = [4]byte{0, 0, 0, 0}
	h.useDNS1 = true
	h.useDNS2 = true
	h.peerIP = netip.Addr{}
}

// Config returns the negotiated IPv4 configuration.
func (h *IPCPHandler) Config() IPConfig {
	localIP := netip.AddrFrom4(h.reqIP)
	if (!localIP.IsValid() || localIP.IsUnspecified()) && h.fallbackIP.IsValid() {
		localIP = h.fallbackIP
	}
	cfg := IPConfig{
		LocalIP: localIP,
		PeerIP:  h.peerIP,
	}
	for _, raw := range [][4]byte{h.reqDNS1, h.reqDNS2} {
		addr := netip.AddrFrom4(raw)
		if addr.IsValid() && !addr.IsUnspecified() {
			cfg.DNS = append(cfg.DNS, addr)
		}
	}
	return cfg
}

// RawDNSStrings returns the negotiated primary and secondary DNS values (including 0.0.0.0)
// for openfortivpn-compatible log formatting ("Got addresses: [x], ns [d1, d2]").
func (h *IPCPHandler) RawDNSStrings() (string, string) {
	return netip.AddrFrom4(h.reqDNS1).String(), netip.AddrFrom4(h.reqDNS2).String()
}

func (h *IPCPHandler) BuildRequestOptions() []Option {
	opts := []Option{
		{Type: OptIPAddress, Value: h.reqIP[:]},
	}
	if h.useDNS1 {
		opts = append(opts, Option{Type: OptPrimaryDNS, Value: h.reqDNS1[:]})
	}
	if h.useDNS2 {
		opts = append(opts, Option{Type: OptSecondaryDNS, Value: h.reqDNS2[:]})
	}
	return opts
}

func (h *IPCPHandler) ApplyNak(opts []Option) error {
	for _, o := range opts {
		if len(o.Value) != 4 {
			continue
		}
		var v [4]byte
		copy(v[:], o.Value)
		switch o.Type {
		case OptIPAddress:
			if addr := netip.AddrFrom4(v); addr.IsValid() && !addr.IsUnspecified() {
				h.reqIP = v
			}
		case OptPrimaryDNS:
			h.reqDNS1 = v
		case OptSecondaryDNS:
			h.reqDNS2 = v
		}
	}
	return nil
}

func (h *IPCPHandler) ApplyReject(opts []Option) error {
	for _, o := range opts {
		switch o.Type {
		case OptIPAddress:
			return fmt.Errorf("IPCP peer rejected IP-Address option")
		case OptPrimaryDNS:
			h.useDNS1 = false
		case OptSecondaryDNS:
			h.useDNS2 = false
		}
	}
	return nil
}

func (h *IPCPHandler) EvaluatePeerRequest(opts []Option) (PeerEvaluation, []Option, error) {
	var rejected []Option
	for _, o := range opts {
		switch o.Type {
		case OptIPAddress:
			if len(o.Value) != 4 {
				rejected = append(rejected, o)
				continue
			}
			var v [4]byte
			copy(v[:], o.Value)
			h.peerIP = netip.AddrFrom4(v)
		case OptPrimaryDNS:
			if len(o.Value) != 4 {
				rejected = append(rejected, o)
				continue
			}
			var v [4]byte
			copy(v[:], o.Value)
			if addr := netip.AddrFrom4(v); addr.IsValid() && !addr.IsUnspecified() && h.reqDNS1 == [4]byte{0, 0, 0, 0} {
				h.reqDNS1 = v
			}
		case OptSecondaryDNS:
			if len(o.Value) != 4 {
				rejected = append(rejected, o)
				continue
			}
			var v [4]byte
			copy(v[:], o.Value)
			if addr := netip.AddrFrom4(v); addr.IsValid() && !addr.IsUnspecified() && h.reqDNS2 == [4]byte{0, 0, 0, 0} {
				h.reqDNS2 = v
			}
		default:
			rejected = append(rejected, o)
		}
	}
	if len(rejected) > 0 {
		return PeerReject, rejected, nil
	}
	return PeerAck, opts, nil
}

func (h *IPCPHandler) HandleExtraCode(_ Packet, _ func(Packet)) (bool, error) {
	return false, nil
}
