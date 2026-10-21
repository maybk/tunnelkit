package fortivpn

// Phase represents a fine-grained lifecycle stage of a VPN session.
type Phase string

const (
	PhaseConnecting     Phase = "connecting"
	PhaseAuthenticating Phase = "authenticating"
	PhaseWaitingInput   Phase = "waiting_input"
	PhaseNegotiating    Phase = "negotiating"
	PhaseConfiguring    Phase = "configuring"
	PhaseConnected      Phase = "connected"
	PhaseReconnecting   Phase = "reconnecting"
	PhaseDisconnecting  Phase = "disconnecting"
	PhaseDisconnected   Phase = "disconnected"
)

// Event carries a state transition, tunnel metadata, periodic traffic stats,
// interactive prompt, or terminal error emitted by the engine.
type Event struct {
	Phase  Phase       `json:"phase"`
	Info   *TunnelInfo `json:"info,omitempty"`
	Stats  *Stats      `json:"stats,omitempty"`
	Prompt *Prompt     `json:"prompt,omitempty"`
	Err    *Error      `json:"error,omitempty"`
}

// TunnelInfo describes the negotiated parameters of an active VPN tunnel.
type TunnelInfo struct {
	Interface            string   `json:"interface,omitempty"`
	LocalIP              string   `json:"localIp,omitempty"`
	PeerIP               string   `json:"peerIp,omitempty"`
	MTU                  int      `json:"mtu,omitempty"`
	RouteCount           int      `json:"routeCount"`
	Routes               []string `json:"routes,omitempty"`
	DNSMode              DNSMode  `json:"dnsMode,omitempty"`
	DNSServers           []string `json:"dnsServers,omitempty"`
	DNSDomains           []string `json:"dnsDomains,omitempty"`
	SplitDNSNeedsDomains bool     `json:"splitDnsNeedsDomains,omitempty"`
	GatewayHost          string   `json:"gatewayHost,omitempty"`
	GatewayIP            string   `json:"gatewayIp,omitempty"`
	FortiOS              string   `json:"fortiOs,omitempty"`
}

// Stats holds cumulative traffic counters and the latest LCP Echo round-trip time.
type Stats struct {
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	RxPackets uint64 `json:"rxPackets"`
	TxPackets uint64 `json:"txPackets"`
	RTTMillis int64  `json:"rttMillis"`
}

// PromptKind distinguishes interactive challenge types during authentication.
type PromptKind string

const (
	PromptOTP     PromptKind = "otp"
	PromptFTMPush PromptKind = "ftm_push"
	PromptSAML    PromptKind = "saml"
)

// Prompt requests interactive user input (OTP code, FortiToken Mobile push, or SAML login).
type Prompt struct {
	ID      string     `json:"id"`
	Kind    PromptKind `json:"kind"`
	Message string     `json:"message,omitempty"`
	URL     string     `json:"url,omitempty"`
}
