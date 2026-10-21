// Package netcfg plans, applies, updates, and rolls back host network
// configuration (interface IP/MTU, split/full routes, gateway host route, and
// split/full DNS) across macOS and Windows.
package netcfg

import (
	"context"
	"net/netip"

	"tunnelkit/fortivpn"
)

// MaxRoutes bounds the total number of tunnel routes installed on the host.
const MaxRoutes = 1024

// DNSPlan describes the desired DNS configuration for the active tunnel.
type DNSPlan struct {
	Mode    fortivpn.DNSMode
	Servers []netip.Addr
	Domains []string
}

// Desired describes the target host network state for a connected VPN session.
type Desired struct {
	Iface    string
	Local    netip.Addr
	Peer     netip.Addr
	MTU      int
	Routes   []netip.Prefix
	Excludes []netip.Prefix
	Gateway  netip.Addr
	DNS      DNSPlan
}

// Snapshot captures pre-connection host routing details needed to pin the
// gateway host route before installing overlapping tunnel routes.
type Snapshot struct {
	GatewayNextHop netip.Addr
	GatewayIface   string
	GatewayLUID    uint64
}

// StepKind identifies a planned host network action.
type StepKind string

const (
	StepConfigureInterface StepKind = "configure_interface"
	StepHostRoute          StepKind = "host_route"
	StepTunnelRoute        StepKind = "tunnel_route"
	StepDNS                StepKind = "dns"
)

// Step is one deterministic action produced by Plan.
type Step struct {
	Kind    StepKind
	Iface   string
	Local   netip.Addr
	Peer    netip.Addr
	MTU     int
	Prefix  netip.Prefix
	NextHop netip.Addr
	DNS     DNSPlan
}

// Applied represents an active host network configuration that can be updated
// across reconnects or rolled back on disconnect.
type Applied interface {
	Update(ctx context.Context, d Desired) error
	Close(ctx context.Context) error
}
