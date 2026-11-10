//go:build linux

package netcfg

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

var linuxOps = unixPlatformOps{
	name:        "linux",
	lookupRoute: lookupLinuxRoute,
	configureInterface: func(ctx context.Context, run cmdRunner, _ Desired, next Desired) error {
		return configureLinuxInterface(ctx, run, next.Iface, next.Local, next.Peer, next.MTU)
	},
	addHostRoute:      addLinuxHostRoute,
	deleteHostRoute:   deleteLinuxHostRoute,
	addTunnelRoute:    addLinuxTunnelRoute,
	deleteTunnelRoute: deleteLinuxTunnelRoute,
	applyDNS:          applyLinuxDNS,
	removeDNS:         removeLinuxDNS,
	dnsJournalKind:    KindLinuxDNS,
}

// Apply configures the Linux TUN interface, gateway host route, tunnel routes,
// and systemd-resolved / resolvconf DNS settings, recording persistent mutations
// to journalPath first.
func Apply(ctx context.Context, d Desired, journalPath string) (Applied, error) {
	return applyLinuxWithRunner(ctx, d, journalPath, defaultCmdRunner)
}

// Recover rolls back any residual host routes or DNS configurations recorded in
// journalPath from a previously crashed helper process.
func Recover(ctx context.Context, journalPath string) error {
	return recoverLinuxWithRunner(ctx, journalPath, defaultCmdRunner)
}

func applyLinuxWithRunner(ctx context.Context, d Desired, journalPath string, run cmdRunner) (Applied, error) {
	return applyUnixWithOps(ctx, d, journalPath, run, linuxOps)
}

func recoverLinuxWithRunner(ctx context.Context, journalPath string, run cmdRunner) error {
	return recoverUnixWithOps(ctx, journalPath, run, linuxOps)
}

func configureLinuxInterface(ctx context.Context, run cmdRunner, iface string, local, peer netip.Addr, mtu int) error {
	_, _ = run(ctx, "", "ip", "-4", "addr", "flush", "dev", iface)
	addrArgs := []string{"-4", "addr", "replace", local.String() + "/32"}
	if peer.IsValid() && !peer.IsUnspecified() {
		addrArgs = append(addrArgs, "peer", peer.String())
	}
	addrArgs = append(addrArgs, "dev", iface)
	if _, err := run(ctx, "", "ip", addrArgs...); err != nil {
		return err
	}
	_, err := run(ctx, "", "ip", "link", "set", "dev", iface, "mtu", strconv.Itoa(mtu), "up")
	return err
}

func lookupLinuxRoute(ctx context.Context, run cmdRunner, gateway netip.Addr, skipIface string) (Snapshot, error) {
	out, err := run(ctx, "", "ip", "-4", "route", "get", gateway.String())
	if err == nil {
		if snap := parseLinuxRouteGet(out, skipIface); snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" {
			return snap, nil
		}
	}
	outDef, errDef := run(ctx, "", "ip", "-4", "route", "show", "default")
	if errDef != nil {
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{}, errDef
	}
	return parseLinuxRouteGet(outDef, skipIface), nil
}

func parseLinuxRouteGet(out string, skipIface string) Snapshot {
	var fallback Snapshot
	for _, line := range strings.Split(out, "\n") {
		var lineSnap Snapshot
		fields := strings.Fields(line)
		for i := 0; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				if !lineSnap.GatewayNextHop.IsValid() {
					if addr, err := netip.ParseAddr(fields[i+1]); err == nil && addr.Is4() {
						lineSnap.GatewayNextHop = addr
					}
				}
			case "dev":
				if lineSnap.GatewayIface == "" {
					lineSnap.GatewayIface = fields[i+1]
				}
			}
		}
		if skipIface != "" && lineSnap.GatewayIface == skipIface {
			continue
		}
		if lineSnap.GatewayIface == "lo" {
			continue
		}
		if lineSnap.GatewayNextHop.IsValid() {
			return lineSnap
		}
		if lineSnap.GatewayIface != "" && fallback.GatewayIface == "" {
			fallback = lineSnap
		}
	}
	return fallback
}

func addLinuxHostRoute(ctx context.Context, run cmdRunner, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
	args := []string{"-4", "route", "add", host.String() + "/32"}
	if nextHop.IsValid() {
		args = append(args, "via", nextHop.String())
	}
	if iface != "" {
		args = append(args, "dev", iface)
	}
	if !nextHop.IsValid() && iface == "" {
		return false, fmt.Errorf("missing nextHop or interface for host route %s", host)
	}
	out, err := run(ctx, "", "ip", args...)
	if err != nil {
		if isRouteAlreadyExists(out, err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func deleteLinuxHostRoute(ctx context.Context, run cmdRunner, host string, nextHop string, iface string) error {
	if host == "" {
		return nil
	}
	target := host
	if !strings.Contains(target, "/") {
		target += "/32"
	}
	args := []string{"-4", "route", "del", target}
	if nextHop != "" {
		args = append(args, "via", nextHop)
	}
	if iface != "" {
		args = append(args, "dev", iface)
	}
	out, err := run(ctx, "", "ip", args...)
	if err != nil && !isRouteNotInTable(out, err) {
		return err
	}
	return nil
}

func addLinuxTunnelRoute(ctx context.Context, run cmdRunner, prefix netip.Prefix, iface string) (bool, error) {
	out, err := run(ctx, "", "ip", "-4", "route", "add", prefix.String(), "dev", iface)
	if err != nil {
		if isRouteAlreadyExists(out, err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func deleteLinuxTunnelRoute(ctx context.Context, run cmdRunner, prefix netip.Prefix, iface string) error {
	out, err := run(ctx, "", "ip", "-4", "route", "del", prefix.String(), "dev", iface)
	if err != nil && !isRouteNotInTable(out, err) {
		return err
	}
	return nil
}
