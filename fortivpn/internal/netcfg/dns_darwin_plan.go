package netcfg

import (
	"context"
	"fmt"
	"strings"

	"tunnelkit/fortivpn"
)

const (
	darwinDNSServiceKey  = "State:/Network/Service/TunnelKit/DNS"
	darwinIPv4ServiceKey = "State:/Network/Service/TunnelKit/IPv4"
	// darwinDNSSearchOrder places TunnelKit's dynamic resolver ahead of default
	// Supplemental resolvers (typically 101600+/200000) without colliding with mDNS (300000+).
	darwinDNSSearchOrder = "100000"
)

func buildScutilSetScript(key string, plan DNSPlan) string {
	var b strings.Builder
	b.WriteString("d.init\n")
	b.WriteString("d.add SearchOrder # ")
	b.WriteString(darwinDNSSearchOrder)
	b.WriteByte('\n')

	addrs := make([]string, 0, len(plan.Servers))
	for _, s := range plan.Servers {
		if s.IsValid() && !s.IsUnspecified() {
			addrs = append(addrs, s.String())
		}
	}
	b.WriteString("d.add ServerAddresses * ")
	b.WriteString(strings.Join(addrs, " "))
	b.WriteByte('\n')

	switch plan.Mode {
	case fortivpn.DNSSplit:
		b.WriteString("d.add SupplementalMatchDomains * ")
		b.WriteString(strings.Join(plan.Domains, " "))
		b.WriteByte('\n')
		b.WriteString("d.add SupplementalMatchDomainsNoSearch # 1\n")
	case fortivpn.DNSFull:
		b.WriteString("d.add SupplementalMatchDomains * \"\"\n")
		if len(plan.Domains) > 0 {
			b.WriteString("d.add SearchDomains * ")
			b.WriteString(strings.Join(plan.Domains, " "))
			b.WriteByte('\n')
		}
	}

	b.WriteString("set ")
	b.WriteString(key)
	b.WriteByte('\n')

	if key == darwinDNSServiceKey {
		if plan.Mode == fortivpn.DNSFull {
			b.WriteString("d.init\n")
			b.WriteString("d.add OverridePrimary # 1\n")
			b.WriteString("set ")
			b.WriteString(darwinIPv4ServiceKey)
			b.WriteByte('\n')
		} else {
			b.WriteString("remove ")
			b.WriteString(darwinIPv4ServiceKey)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func buildScutilRemoveScript(key string) string {
	if key == darwinDNSServiceKey {
		return fmt.Sprintf("remove %s\nremove %s\n", darwinDNSServiceKey, darwinIPv4ServiceKey)
	}
	return fmt.Sprintf("remove %s\n", key)
}

func applyDarwinDNS(ctx context.Context, run cmdRunner, key string, plan DNSPlan) error {
	if !HasEffectiveDNS(plan) {
		return nil
	}
	script := buildScutilSetScript(key, plan)
	out, err := run(ctx, script, "/usr/sbin/scutil")
	if err != nil {
		return fmt.Errorf("scutil set %s: %w", key, err)
	}
	for line := range strings.SplitSeq(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Contains(strings.ToLower(trimmed), "no such key") {
			continue
		}
		return fmt.Errorf("scutil set %s: %s", key, trimmed)
	}
	flushDarwinDNSCache(ctx, run)
	return nil
}

// ensureDarwinDNS verifies that macOS configd has not wiped TunnelKit's volatile
// Dynamic Store DNS entries across sleep/wake or network transitions, restoring them if missing.
func ensureDarwinDNS(ctx context.Context, run cmdRunner, key string, plan DNSPlan) error {
	if !HasEffectiveDNS(plan) {
		return nil
	}
	if key == "" {
		key = darwinDNSServiceKey
	}
	out, err := run(ctx, fmt.Sprintf("show %s\nquit\n", key), "/usr/sbin/scutil")
	if err == nil && darwinScutilDNSMatches(out, plan) {
		if key != darwinDNSServiceKey || plan.Mode != fortivpn.DNSFull {
			return nil
		}
		ipv4Out, ipv4Err := run(ctx, fmt.Sprintf("show %s\nquit\n", darwinIPv4ServiceKey), "/usr/sbin/scutil")
		if ipv4Err == nil && !strings.Contains(strings.ToLower(ipv4Out), "no such key") && strings.Contains(ipv4Out, "OverridePrimary") {
			return nil
		}
	}
	return applyDarwinDNS(ctx, run, key, plan)
}

func darwinScutilDNSMatches(out string, plan DNSPlan) bool {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "no such key") || !strings.Contains(out, "SearchOrder") || !strings.Contains(out, darwinDNSSearchOrder) {
		return false
	}
	for _, s := range plan.Servers {
		if s.IsValid() && !s.IsUnspecified() && !strings.Contains(out, s.String()) {
			return false
		}
	}
	if plan.Mode == fortivpn.DNSSplit {
		for _, d := range plan.Domains {
			if trimmed := strings.TrimSpace(d); trimmed != "" && !strings.Contains(out, trimmed) {
				return false
			}
		}
	}
	return true
}

func removeDarwinDNS(ctx context.Context, run cmdRunner, key string) error {
	if key == "" {
		key = darwinDNSServiceKey
	}
	script := buildScutilRemoveScript(key)
	out, err := run(ctx, script, "/usr/sbin/scutil")
	if err != nil {
		return fmt.Errorf("scutil remove %s: %w", key, err)
	}
	for line := range strings.SplitSeq(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Contains(strings.ToLower(trimmed), "no such key") {
			continue
		}
		return fmt.Errorf("scutil remove %s: %s", key, trimmed)
	}
	flushDarwinDNSCache(ctx, run)
	return nil
}

func flushDarwinDNSCache(ctx context.Context, run cmdRunner) {
	_, _ = run(ctx, "", "/usr/bin/dscacheutil", "-flushcache")
	_, _ = run(ctx, "", "/usr/bin/killall", "-HUP", "mDNSResponder")
}
