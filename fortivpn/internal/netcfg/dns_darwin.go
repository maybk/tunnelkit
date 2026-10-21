//go:build darwin

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
)

func buildScutilSetScript(key string, plan DNSPlan) string {
	var b strings.Builder
	b.WriteString("d.init\n")

	addrs := make([]string, 0, len(plan.Servers))
	for _, s := range plan.Servers {
		if s.IsValid() {
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

	if plan.Mode == fortivpn.DNSFull && key == darwinDNSServiceKey {
		b.WriteString("d.init\n")
		b.WriteString("d.add OverridePrimary # 1\n")
		b.WriteString("set ")
		b.WriteString(darwinIPv4ServiceKey)
		b.WriteByte('\n')
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
	if plan.Mode == fortivpn.DNSOff || len(plan.Servers) == 0 || SplitDNSNeedsDomains(plan) {
		return nil
	}
	script := buildScutilSetScript(key, plan)
	out, err := run(ctx, script, "/usr/sbin/scutil")
	if err != nil {
		return fmt.Errorf("scutil set %s: %w", key, err)
	}
	if trimmed := strings.TrimSpace(out); trimmed != "" {
		return fmt.Errorf("scutil set %s: %s", key, trimmed)
	}
	flushDarwinDNSCache(ctx, run)
	return nil
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
	for _, line := range strings.Split(out, "\n") {
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
