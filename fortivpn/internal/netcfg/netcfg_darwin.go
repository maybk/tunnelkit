//go:build darwin

package netcfg

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

var darwinOps = unixPlatformOps{
	name:               "darwin",
	lookupRoute:        lookupDarwinRoute,
	configureInterface: configureDarwinInterface,
	addHostRoute:       addDarwinHostRoute,
	deleteHostRoute:    deleteDarwinHostRoute,
	addTunnelRoute:     addDarwinTunnelRoute,
	deleteTunnelRoute:  deleteDarwinTunnelRoute,
	applyDNS: func(ctx context.Context, run cmdRunner, _ string, plan DNSPlan, _ *Journal) (string, error) {
		if err := applyDarwinDNS(ctx, run, darwinDNSServiceKey, plan); err != nil {
			return "", err
		}
		return darwinDNSServiceKey, nil
	},
	removeDNS: func(ctx context.Context, run cmdRunner, _ string, key string) error {
		return removeDarwinDNS(ctx, run, key)
	},
	dnsJournalKind: KindDarwinDNSKey,
	dnsStaticKey:   darwinDNSServiceKey,
}

// Apply configures the macOS utun interface, gateway host route, tunnel routes,
// and scutil DNS settings, recording persistent mutations to journalPath first.
func Apply(ctx context.Context, d Desired, journalPath string) (Applied, error) {
	return applyWithRunner(ctx, d, journalPath, defaultCmdRunner)
}

// Recover rolls back any residual host routes or scutil DNS keys recorded in
// journalPath from a previously crashed helper process.
func Recover(ctx context.Context, journalPath string) error {
	return recoverWithRunner(ctx, journalPath, defaultCmdRunner)
}

func applyWithRunner(ctx context.Context, d Desired, journalPath string, run cmdRunner) (Applied, error) {
	return applyUnixWithOps(ctx, d, journalPath, run, darwinOps)
}

func recoverWithRunner(ctx context.Context, journalPath string, run cmdRunner) error {
	return recoverUnixWithOps(ctx, journalPath, run, darwinOps)
}

func configureDarwinInterface(ctx context.Context, run cmdRunner, prev Desired, next Desired) error {
	if prev.Local.IsValid() && (next.Local != prev.Local || next.Peer != prev.Peer) {
		_, _ = run(ctx, "", "/sbin/ifconfig", next.Iface, "inet", prev.Local.String(), prev.Peer.String(), "delete")
	}
	_, err := run(
		ctx,
		"",
		"/sbin/ifconfig",
		next.Iface,
		"inet",
		next.Local.String(),
		next.Peer.String(),
		"netmask",
		"255.255.255.255",
		"mtu",
		strconv.Itoa(next.MTU),
		"up",
	)
	return err
}

func lookupDarwinRoute(ctx context.Context, run cmdRunner, gateway netip.Addr, skipIface string) (Snapshot, error) {
	out, err := run(ctx, "", "/sbin/route", "-n", "get", gateway.String())
	if err == nil {
		snap := parseDarwinRouteGet(out)
		if (snap.GatewayNextHop.IsValid() || snap.GatewayIface != "") && (skipIface == "" || snap.GatewayIface != skipIface) {
			return snap, nil
		}
	}
	outDef, errDef := run(ctx, "", "/sbin/route", "-n", "get", "default")
	if errDef != nil {
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{}, errDef
	}
	return parseDarwinRouteGet(outDef), nil
}

func parseDarwinRouteGet(out string) Snapshot {
	var snap Snapshot
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, "gateway:"); ok {
			if addr, err := netip.ParseAddr(strings.TrimSpace(after)); err == nil && addr.Is4() {
				snap.GatewayNextHop = addr
			}
		} else if after, ok := strings.CutPrefix(trimmed, "interface:"); ok {
			snap.GatewayIface = strings.TrimSpace(after)
		}
	}
	return snap
}

func addDarwinHostRoute(ctx context.Context, run cmdRunner, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
	var args []string
	if nextHop.IsValid() {
		args = []string{"-n", "add", "-host", host.String(), nextHop.String()}
	} else if iface != "" {
		args = []string{"-n", "add", "-host", host.String(), "-interface", iface}
	} else {
		return false, fmt.Errorf("missing nextHop or interface for host route %s", host)
	}
	out, err := run(ctx, "", "/sbin/route", args...)
	if err != nil {
		if isRouteAlreadyExists(out, err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func deleteDarwinHostRoute(ctx context.Context, run cmdRunner, host string, nextHop string, iface string) error {
	if host == "" {
		return nil
	}
	var args []string
	switch {
	case nextHop != "":
		args = []string{"-n", "delete", "-host", host, nextHop}
	case iface != "":
		args = []string{"-n", "delete", "-host", host, "-interface", iface}
	default:
		args = []string{"-n", "delete", "-host", host}
	}
	out, err := run(ctx, "", "/sbin/route", args...)
	if err != nil && !isRouteNotInTable(out, err) {
		return err
	}
	return nil
}

func addDarwinTunnelRoute(ctx context.Context, run cmdRunner, prefix netip.Prefix, iface string) (bool, error) {
	out, err := run(ctx, "", "/sbin/route", "-n", "add", "-net", prefix.String(), "-interface", iface)
	if err != nil {
		if isRouteAlreadyExists(out, err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func deleteDarwinTunnelRoute(ctx context.Context, run cmdRunner, prefix netip.Prefix, iface string) error {
	out, err := run(ctx, "", "/sbin/route", "-n", "delete", "-net", prefix.String(), "-interface", iface)
	if err != nil && !isRouteNotInTable(out, err) {
		return err
	}
	return nil
}
