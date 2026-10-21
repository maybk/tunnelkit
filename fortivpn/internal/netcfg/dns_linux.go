//go:build linux

package netcfg

import (
	"context"
	"fmt"
	"strings"

	"tunnelkit/fortivpn"
)

const (
	linuxDNSBackendResolvectl = "resolvectl"
	linuxDNSBackendResolvconf = "resolvconf"
)

func applyLinuxDNS(ctx context.Context, run cmdRunner, iface string, plan DNSPlan, journal *Journal) (string, error) {
	if plan.Mode == fortivpn.DNSOff || len(plan.Servers) == 0 || SplitDNSNeedsDomains(plan) {
		return "", nil
	}

	var servers []string
	for _, s := range plan.Servers {
		if s.IsValid() && !s.IsUnspecified() {
			servers = append(servers, s.String())
		}
	}
	if len(servers) == 0 {
		return "", nil
	}

	// 1. Prefer systemd-resolved via resolvectl (per-link DNS + routing domains).
	entry := JournalEntry{
		Kind:  KindLinuxDNS,
		Iface: iface,
		Key:   linuxDNSBackendResolvectl,
	}
	if err := journal.Record(entry); err != nil {
		return "", err
	}

	dnsArgs := append([]string{"dns", iface}, servers...)
	if _, err := run(ctx, "", "resolvectl", dnsArgs...); err == nil {
		var domainArgs []string
		switch plan.Mode {
		case fortivpn.DNSSplit:
			domainArgs = []string{"domain", iface}
			for _, d := range plan.Domains {
				trimmed := strings.TrimPrefix(strings.TrimSpace(d), ".")
				if trimmed != "" {
					domainArgs = append(domainArgs, "~"+trimmed)
				}
			}
			_, _ = run(ctx, "", "resolvectl", "default-route", iface, "false")
		case fortivpn.DNSFull:
			domainArgs = []string{"domain", iface, "~."}
			for _, d := range plan.Domains {
				trimmed := strings.TrimPrefix(strings.TrimSpace(d), ".")
				if trimmed != "" {
					domainArgs = append(domainArgs, trimmed)
				}
			}
			_, _ = run(ctx, "", "resolvectl", "default-route", iface, "true")
		}
		if len(domainArgs) > 2 {
			if _, err := run(ctx, "", "resolvectl", domainArgs...); err != nil {
				return "", fmt.Errorf("resolvectl domain %s: %w", iface, err)
			}
		}
		_, _ = run(ctx, "", "resolvectl", "flush-caches")
		return linuxDNSBackendResolvectl, nil
	}

	// 2. Fallback to openresolv / resolvconf if systemd-resolved is unavailable.
	_ = journal.Remove(entry)
	rcEntry := JournalEntry{
		Kind:  KindLinuxDNS,
		Iface: iface,
		Key:   linuxDNSBackendResolvconf,
	}
	if err := journal.Record(rcEntry); err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, s := range servers {
		sb.WriteString("nameserver ")
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	if len(plan.Domains) > 0 {
		sb.WriteString("search ")
		sb.WriteString(strings.Join(plan.Domains, " "))
		sb.WriteByte('\n')
	}
	if _, err := run(ctx, sb.String(), "resolvconf", "-a", iface+".tunnelkit"); err != nil {
		_ = journal.Remove(rcEntry)
		return "", fmt.Errorf("linux DNS configuration failed (neither resolvectl nor resolvconf succeeded): %w", err)
	}
	return linuxDNSBackendResolvconf, nil
}

func removeLinuxDNS(ctx context.Context, run cmdRunner, iface string, backend string) error {
	if iface == "" {
		return nil
	}
	switch backend {
	case linuxDNSBackendResolvconf:
		out, err := run(ctx, "", "resolvconf", "-d", iface+".tunnelkit")
		if err != nil && !isRouteNotInTable(out, err) {
			return err
		}
		return nil
	default:
		out, err := run(ctx, "", "resolvectl", "revert", iface)
		if err != nil {
			lower := strings.ToLower(out + " " + err.Error())
			if strings.Contains(lower, "unknown interface") ||
				strings.Contains(lower, "no such device") ||
				strings.Contains(lower, "not found") {
				return nil
			}
			return err
		}
		_, _ = run(ctx, "", "resolvectl", "flush-caches")
		return nil
	}
}
