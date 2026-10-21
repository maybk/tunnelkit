//go:build linux

package netcfg

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"tunnelkit/fortivpn"
)

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

type linuxApplied struct {
	mu           sync.Mutex
	run          cmdRunner
	journal      *Journal
	desired      Desired
	snap         Snapshot
	tunnelRoutes []netip.Prefix
	dnsBackend   string
}

func applyLinuxWithRunner(ctx context.Context, d Desired, journalPath string, run cmdRunner) (Applied, error) {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: load journal failed", err)
	}

	var snap Snapshot
	if NeedsGatewayHostRoute(d.Routes, d.Gateway) {
		snap, err = lookupLinuxRoute(ctx, run, d.Gateway)
		if err != nil {
			return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: lookup gateway route failed", err)
		}
	}

	steps := Plan(d, snap)
	ap := &linuxApplied{
		run:     run,
		journal: journal,
		desired: d,
		snap:    snap,
	}

	if err := ap.executeSteps(ctx, steps); err != nil {
		_ = ap.Close(context.Background())
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: apply linux network configuration failed", err)
	}
	return ap, nil
}

func (a *linuxApplied) executeSteps(ctx context.Context, steps []Step) error {
	for _, s := range steps {
		switch s.Kind {
		case StepConfigureInterface:
			if err := configureLinuxInterface(ctx, a.run, s.Iface, s.Local, s.Peer, s.MTU); err != nil {
				return err
			}

		case StepHostRoute:
			entry := JournalEntry{
				Kind:    KindHostRoute,
				Gateway: s.Prefix.Addr().String(),
				Iface:   s.Iface,
			}
			if s.NextHop.IsValid() {
				entry.NextHop = s.NextHop.String()
			}
			if err := a.journal.Record(entry); err != nil {
				return err
			}
			created, err := addLinuxHostRoute(ctx, a.run, s.Prefix.Addr(), s.NextHop, s.Iface)
			if err != nil {
				_ = a.journal.Remove(entry)
				return err
			}
			if !created {
				_ = a.journal.Remove(entry)
			}

		case StepTunnelRoute:
			created, err := addLinuxTunnelRoute(ctx, a.run, s.Prefix, s.Iface)
			if err != nil {
				return err
			}
			if created {
				a.tunnelRoutes = append(a.tunnelRoutes, s.Prefix)
			}

		case StepDNS:
			backend, err := applyLinuxDNS(ctx, a.run, s.Iface, s.DNS, a.journal)
			if err != nil {
				return err
			}
			a.dnsBackend = backend
		}
	}
	return nil
}

func (a *linuxApplied) Update(ctx context.Context, d Desired) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	mtu := d.MTU
	if mtu <= 0 {
		mtu = 1400
	}
	peer := d.Peer
	if !peer.IsValid() || peer.IsUnspecified() || peer == d.Gateway || peer == d.Local {
		peer = netip.MustParseAddr("169.254.2.1")
	}

	if d.Local != a.desired.Local || peer != a.desired.Peer || mtu != a.desired.MTU {
		if err := configureLinuxInterface(ctx, a.run, d.Iface, d.Local, peer, mtu); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update interface failed", err)
		}
	}

	oldSet := make(map[netip.Prefix]struct{}, len(a.tunnelRoutes))
	for _, r := range a.tunnelRoutes {
		oldSet[r] = struct{}{}
	}
	newSet := make(map[netip.Prefix]struct{}, len(d.Routes))
	for _, r := range d.Routes {
		newSet[r] = struct{}{}
	}

	var updatedRoutes []netip.Prefix
	for _, r := range a.tunnelRoutes {
		if _, keep := newSet[r]; !keep {
			_ = deleteLinuxTunnelRoute(ctx, a.run, r, a.desired.Iface)
		} else {
			updatedRoutes = append(updatedRoutes, r)
		}
	}
	for _, r := range d.Routes {
		if _, exists := oldSet[r]; !exists {
			created, err := addLinuxTunnelRoute(ctx, a.run, r, d.Iface)
			if err != nil {
				return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: add updated route failed", err)
			}
			if created {
				updatedRoutes = append(updatedRoutes, r)
			}
		}
	}
	a.tunnelRoutes = updatedRoutes

	if d.DNS.Mode != fortivpn.DNSOff && len(d.DNS.Servers) > 0 && !SplitDNSNeedsDomains(d.DNS) {
		backend, err := applyLinuxDNS(ctx, a.run, d.Iface, d.DNS, a.journal)
		if err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
		a.dnsBackend = backend
	} else if a.dnsBackend != "" {
		_ = removeLinuxDNS(ctx, a.run, d.Iface, a.dnsBackend)
		a.dnsBackend = ""
	}

	a.desired = d
	return nil
}

func (a *linuxApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var errs []error
	for i := len(a.tunnelRoutes) - 1; i >= 0; i-- {
		_ = deleteLinuxTunnelRoute(ctx, a.run, a.tunnelRoutes[i], a.desired.Iface)
	}
	a.tunnelRoutes = nil

	if err := rollbackLinuxJournalEntries(ctx, a.run, a.journal); err != nil {
		errs = append(errs, err)
	}
	if err := a.journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func recoverLinuxWithRunner(ctx context.Context, journalPath string, run cmdRunner) error {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return err
	}
	var errs []error
	if err := rollbackLinuxJournalEntries(ctx, run, journal); err != nil {
		errs = append(errs, err)
	}
	if err := journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func rollbackLinuxJournalEntries(ctx context.Context, run cmdRunner, journal *Journal) error {
	var errs []error
	for _, entry := range journal.Entries() {
		switch entry.Kind {
		case KindLinuxDNS:
			if err := removeLinuxDNS(ctx, run, entry.Iface, entry.Key); err != nil {
				errs = append(errs, err)
			}
		case KindHostRoute:
			if err := deleteLinuxHostRoute(ctx, run, entry.Gateway, entry.NextHop, entry.Iface); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func configureLinuxInterface(ctx context.Context, run cmdRunner, iface string, local, peer netip.Addr, mtu int) error {
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

func lookupLinuxRoute(ctx context.Context, run cmdRunner, gateway netip.Addr) (Snapshot, error) {
	out, err := run(ctx, "", "ip", "-4", "route", "get", gateway.String())
	if err == nil {
		if snap := parseLinuxRouteGet(out); snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" {
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
	return parseLinuxRouteGet(outDef), nil
}

func parseLinuxRouteGet(out string) Snapshot {
	var snap Snapshot
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i := 0; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				if !snap.GatewayNextHop.IsValid() {
					if addr, err := netip.ParseAddr(fields[i+1]); err == nil && addr.Is4() {
						snap.GatewayNextHop = addr
					}
				}
			case "dev":
				if snap.GatewayIface == "" {
					snap.GatewayIface = fields[i+1]
				}
			}
		}
		if snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" {
			return snap
		}
	}
	return snap
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
