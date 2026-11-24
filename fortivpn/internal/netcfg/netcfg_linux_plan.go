package netcfg

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// linuxNetlinkOps abstracts Linux interface and IPv4 route operations so production
// code talks directly to the kernel via Netlink while cross-platform tests can
// inject mocks or command runners on any host OS.
type linuxNetlinkOps interface {
	configureInterface(ctx context.Context, iface string, local, peer netip.Addr, mtu int) error
	lookupRoute(ctx context.Context, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error)
	addHostRoute(ctx context.Context, host netip.Addr, nextHop netip.Addr, iface string) (bool, error)
	deleteHostRoute(ctx context.Context, host string, nextHop string, iface string) error
	addTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) (bool, error)
	deleteTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) error
	close() error
}

func makeLinuxPlatformOps(nlOps linuxNetlinkOps) unixPlatformOps {
	return unixPlatformOps{
		name: "linux",
		lookupRoute: func(ctx context.Context, _ cmdRunner, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
			return nlOps.lookupRoute(ctx, gateway, skipIface, skipExistingHostRoute)
		},
		configureInterface: func(ctx context.Context, _ cmdRunner, _ Desired, next Desired) error {
			return nlOps.configureInterface(ctx, next.Iface, next.Local, next.Peer, next.MTU)
		},
		addHostRoute: func(ctx context.Context, _ cmdRunner, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
			return nlOps.addHostRoute(ctx, host, nextHop, iface)
		},
		deleteHostRoute: func(ctx context.Context, _ cmdRunner, host string, nextHop string, iface string) error {
			return nlOps.deleteHostRoute(ctx, host, nextHop, iface)
		},
		addTunnelRoute: func(ctx context.Context, _ cmdRunner, prefix netip.Prefix, iface string) (bool, error) {
			return nlOps.addTunnelRoute(ctx, prefix, iface)
		},
		deleteTunnelRoute: func(ctx context.Context, _ cmdRunner, prefix netip.Prefix, iface string) error {
			return nlOps.deleteTunnelRoute(ctx, prefix, iface)
		},
		applyDNS:       applyLinuxDNS,
		removeDNS:      removeLinuxDNS,
		ensureDNS:      ensureLinuxDNS,
		closeOps:       nlOps.close,
		dnsJournalKind: KindLinuxDNS,
	}
}

func applyLinuxWithOps(ctx context.Context, d Desired, journalPath string, nlOps linuxNetlinkOps, run cmdRunner) (Applied, error) {
	if nlOps == nil {
		nlOps = linuxCmdOps{run: run}
	}
	return applyUnixWithOps(ctx, d, journalPath, run, makeLinuxPlatformOps(nlOps))
}

func recoverLinuxWithOps(ctx context.Context, journalPath string, nlOps linuxNetlinkOps, run cmdRunner) error {
	if nlOps == nil {
		nlOps = linuxCmdOps{run: run}
	}
	defer func() { _ = nlOps.close() }()
	return recoverUnixWithOps(ctx, journalPath, run, makeLinuxPlatformOps(nlOps))
}

func applyLinuxWithRunner(ctx context.Context, d Desired, journalPath string, run cmdRunner) (Applied, error) {
	return applyLinuxWithOps(ctx, d, journalPath, linuxCmdOps{run: run}, run)
}

func recoverLinuxWithRunner(ctx context.Context, journalPath string, run cmdRunner) error {
	return recoverLinuxWithOps(ctx, journalPath, linuxCmdOps{run: run}, run)
}

type linuxCmdOps struct {
	run cmdRunner
}

func (o linuxCmdOps) configureInterface(ctx context.Context, iface string, local, peer netip.Addr, mtu int) error {
	return configureLinuxInterface(ctx, o.run, iface, local, peer, mtu)
}

func (o linuxCmdOps) lookupRoute(ctx context.Context, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
	return lookupLinuxRoute(ctx, o.run, gateway, skipIface, skipExistingHostRoute)
}

func (o linuxCmdOps) addHostRoute(ctx context.Context, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
	return addLinuxHostRoute(ctx, o.run, host, nextHop, iface)
}

func (o linuxCmdOps) deleteHostRoute(ctx context.Context, host string, nextHop string, iface string) error {
	return deleteLinuxHostRoute(ctx, o.run, host, nextHop, iface)
}

func (o linuxCmdOps) addTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) (bool, error) {
	return addLinuxTunnelRoute(ctx, o.run, prefix, iface)
}

func (o linuxCmdOps) deleteTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) error {
	return deleteLinuxTunnelRoute(ctx, o.run, prefix, iface)
}

func (linuxCmdOps) close() error { return nil }

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

func lookupLinuxRoute(ctx context.Context, run cmdRunner, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
	var err error
	if !skipExistingHostRoute {
		var out string
		out, err = run(ctx, "", "ip", "-4", "route", "get", gateway.String())
		if err == nil {
			if snap := parseLinuxRouteGet(out, skipIface); snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" {
				return snap, nil
			}
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
	for line := range strings.SplitSeq(out, "\n") {
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
