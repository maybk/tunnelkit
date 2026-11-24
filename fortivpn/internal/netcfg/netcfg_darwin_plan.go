package netcfg

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// darwinRouteOps abstracts macOS route table lookup and mutations so production
// code uses native PF_ROUTE sockets while cross-platform tests can inject mocks
// or command-based runners without root privileges.
type darwinRouteOps interface {
	lookupRoute(ctx context.Context, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error)
	addHostRoute(ctx context.Context, host netip.Addr, nextHop netip.Addr, iface string) (bool, error)
	deleteHostRoute(ctx context.Context, host string, nextHop string, iface string) error
	addTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) (bool, error)
	deleteTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) error
	close() error
}

func makeDarwinPlatformOps(routeOps darwinRouteOps) unixPlatformOps {
	return unixPlatformOps{
		name: "darwin",
		lookupRoute: func(ctx context.Context, _ cmdRunner, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
			return routeOps.lookupRoute(ctx, gateway, skipIface, skipExistingHostRoute)
		},
		configureInterface: configureDarwinInterface,
		addHostRoute: func(ctx context.Context, _ cmdRunner, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
			return routeOps.addHostRoute(ctx, host, nextHop, iface)
		},
		deleteHostRoute: func(ctx context.Context, _ cmdRunner, host string, nextHop string, iface string) error {
			return routeOps.deleteHostRoute(ctx, host, nextHop, iface)
		},
		addTunnelRoute: func(ctx context.Context, _ cmdRunner, prefix netip.Prefix, iface string) (bool, error) {
			return routeOps.addTunnelRoute(ctx, prefix, iface)
		},
		deleteTunnelRoute: func(ctx context.Context, _ cmdRunner, prefix netip.Prefix, iface string) error {
			return routeOps.deleteTunnelRoute(ctx, prefix, iface)
		},
		applyDNS: func(ctx context.Context, run cmdRunner, _ string, plan DNSPlan, _ *Journal) (string, error) {
			if err := applyDarwinDNS(ctx, run, darwinDNSServiceKey, plan); err != nil {
				return "", err
			}
			return darwinDNSServiceKey, nil
		},
		removeDNS: func(ctx context.Context, run cmdRunner, _ string, key string) error {
			return removeDarwinDNS(ctx, run, key)
		},
		ensureDNS: func(ctx context.Context, run cmdRunner, _ string, key string, plan DNSPlan, _ *Journal) (string, error) {
			if key == "" {
				key = darwinDNSServiceKey
			}
			if err := ensureDarwinDNS(ctx, run, key, plan); err != nil {
				return "", err
			}
			return key, nil
		},
		closeOps:       routeOps.close,
		dnsJournalKind: KindDarwinDNSKey,
		dnsStaticKey:   darwinDNSServiceKey,
	}
}

func applyDarwinWithOps(ctx context.Context, d Desired, journalPath string, routeOps darwinRouteOps, run cmdRunner) (Applied, error) {
	if routeOps == nil {
		routeOps = darwinCmdRouteOps{run: run}
	}
	return applyUnixWithOps(ctx, d, journalPath, run, makeDarwinPlatformOps(routeOps))
}

func recoverDarwinWithOps(ctx context.Context, journalPath string, routeOps darwinRouteOps, run cmdRunner) error {
	if routeOps == nil {
		routeOps = darwinCmdRouteOps{run: run}
	}
	defer func() { _ = routeOps.close() }()
	return recoverUnixWithOps(ctx, journalPath, run, makeDarwinPlatformOps(routeOps))
}

func applyDarwinWithRunner(ctx context.Context, d Desired, journalPath string, run cmdRunner) (Applied, error) {
	return applyDarwinWithOps(ctx, d, journalPath, darwinCmdRouteOps{run: run}, run)
}

func recoverDarwinWithRunner(ctx context.Context, journalPath string, run cmdRunner) error {
	return recoverDarwinWithOps(ctx, journalPath, darwinCmdRouteOps{run: run}, run)
}

type darwinCmdRouteOps struct {
	run cmdRunner
}

func (o darwinCmdRouteOps) lookupRoute(ctx context.Context, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
	return lookupDarwinRoute(ctx, o.run, gateway, skipIface, skipExistingHostRoute)
}

func (o darwinCmdRouteOps) addHostRoute(ctx context.Context, host netip.Addr, nextHop netip.Addr, iface string) (bool, error) {
	return addDarwinHostRoute(ctx, o.run, host, nextHop, iface)
}

func (o darwinCmdRouteOps) deleteHostRoute(ctx context.Context, host string, nextHop string, iface string) error {
	return deleteDarwinHostRoute(ctx, o.run, host, nextHop, iface)
}

func (o darwinCmdRouteOps) addTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) (bool, error) {
	return addDarwinTunnelRoute(ctx, o.run, prefix, iface)
}

func (o darwinCmdRouteOps) deleteTunnelRoute(ctx context.Context, prefix netip.Prefix, iface string) error {
	return deleteDarwinTunnelRoute(ctx, o.run, prefix, iface)
}

func (darwinCmdRouteOps) close() error { return nil }

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

func lookupDarwinRoute(ctx context.Context, run cmdRunner, gateway netip.Addr, skipIface string, skipExistingHostRoute bool) (Snapshot, error) {
	var err error
	if !skipExistingHostRoute {
		var out string
		out, err = run(ctx, "", "/sbin/route", "-n", "get", gateway.String())
		if err == nil {
			snap := parseDarwinRouteGet(out)
			if (snap.GatewayNextHop.IsValid() || snap.GatewayIface != "") && (skipIface == "" || snap.GatewayIface != skipIface) {
				return snap, nil
			}
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
	for line := range strings.SplitSeq(out, "\n") {
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
