//go:build darwin

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

type darwinApplied struct {
	mu           sync.Mutex
	run          cmdRunner
	journal      *Journal
	desired      Desired
	snap         Snapshot
	tunnelRoutes []netip.Prefix
	dnsApplied   bool
}

func applyWithRunner(ctx context.Context, d Desired, journalPath string, run cmdRunner) (Applied, error) {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: load journal failed", err)
	}

	var snap Snapshot
	if NeedsGatewayHostRoute(d.Routes, d.Gateway) {
		snap, err = lookupDarwinRoute(ctx, run, d.Gateway)
		if err != nil {
			return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: lookup gateway route failed", err)
		}
	}

	steps := Plan(d, snap)
	ap := &darwinApplied{
		run:     run,
		journal: journal,
		desired: d,
		snap:    snap,
	}

	if err := ap.executeSteps(ctx, steps); err != nil {
		_ = ap.Close(context.Background())
		return nil, fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: apply darwin network configuration failed", err)
	}
	return ap, nil
}

func (a *darwinApplied) executeSteps(ctx context.Context, steps []Step) error {
	for _, s := range steps {
		switch s.Kind {
		case StepConfigureInterface:
			_, err := a.run(
				ctx,
				"",
				"/sbin/ifconfig",
				s.Iface,
				"inet",
				s.Local.String(),
				s.Peer.String(),
				"netmask",
				"255.255.255.255",
				"mtu",
				strconv.Itoa(s.MTU),
				"up",
			)
			if err != nil {
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
			created, err := addDarwinHostRoute(ctx, a.run, s.Prefix.Addr(), s.NextHop, s.Iface)
			if err != nil {
				_ = a.journal.Remove(entry)
				return err
			}
			if !created {
				// Route already existed before our session; do not delete on rollback.
				_ = a.journal.Remove(entry)
			}

		case StepTunnelRoute:
			created, err := addDarwinTunnelRoute(ctx, a.run, s.Prefix, s.Iface)
			if err != nil {
				return err
			}
			if created {
				a.tunnelRoutes = append(a.tunnelRoutes, s.Prefix)
			}

		case StepDNS:
			if err := a.journal.Record(JournalEntry{
				Kind: KindDarwinDNSKey,
				Key:  darwinDNSServiceKey,
			}); err != nil {
				return err
			}
			if err := applyDarwinDNS(ctx, a.run, darwinDNSServiceKey, s.DNS); err != nil {
				return err
			}
			a.dnsApplied = true
		}
	}
	return nil
}

func (a *darwinApplied) Update(ctx context.Context, d Desired) error {
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
		if _, err := a.run(
			ctx,
			"",
			"/sbin/ifconfig",
			d.Iface,
			"inet",
			d.Local.String(),
			peer.String(),
			"netmask",
			"255.255.255.255",
			"mtu",
			strconv.Itoa(mtu),
			"up",
		); err != nil {
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
			_ = deleteDarwinTunnelRoute(ctx, a.run, r, a.desired.Iface)
		} else {
			updatedRoutes = append(updatedRoutes, r)
		}
	}
	for _, r := range d.Routes {
		if _, exists := oldSet[r]; !exists {
			created, err := addDarwinTunnelRoute(ctx, a.run, r, d.Iface)
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
		if err := a.journal.Record(JournalEntry{
			Kind: KindDarwinDNSKey,
			Key:  darwinDNSServiceKey,
		}); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: record dns journal failed", err)
		}
		if err := applyDarwinDNS(ctx, a.run, darwinDNSServiceKey, d.DNS); err != nil {
			return fortivpn.WrapError(fortivpn.CodeNetcfgFailed, "netcfg: update dns failed", err)
		}
		a.dnsApplied = true
	} else if a.dnsApplied {
		_ = removeDarwinDNS(ctx, a.run, darwinDNSServiceKey)
		a.dnsApplied = false
	}

	a.desired = d
	return nil
}

func (a *darwinApplied) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	var errs []error
	for i := len(a.tunnelRoutes) - 1; i >= 0; i-- {
		_ = deleteDarwinTunnelRoute(ctx, a.run, a.tunnelRoutes[i], a.desired.Iface)
	}
	a.tunnelRoutes = nil

	if err := rollbackJournalEntries(ctx, a.run, a.journal); err != nil {
		errs = append(errs, err)
	}
	if err := a.journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func recoverWithRunner(ctx context.Context, journalPath string, run cmdRunner) error {
	journal, err := LoadJournal(journalPath)
	if err != nil {
		return err
	}
	var errs []error
	if err := rollbackJournalEntries(ctx, run, journal); err != nil {
		errs = append(errs, err)
	}
	if err := journal.Clear(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func rollbackJournalEntries(ctx context.Context, run cmdRunner, journal *Journal) error {
	var errs []error
	for _, entry := range journal.Entries() {
		switch entry.Kind {
		case KindDarwinDNSKey:
			if err := removeDarwinDNS(ctx, run, entry.Key); err != nil {
				errs = append(errs, err)
			}
		case KindHostRoute:
			if err := deleteDarwinHostRoute(ctx, run, entry.Gateway, entry.NextHop, entry.Iface); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func lookupDarwinRoute(ctx context.Context, run cmdRunner, gateway netip.Addr) (Snapshot, error) {
	out, err := run(ctx, "", "/sbin/route", "-n", "get", gateway.String())
	if err == nil {
		if snap := parseDarwinRouteGet(out); snap.GatewayNextHop.IsValid() || snap.GatewayIface != "" {
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
