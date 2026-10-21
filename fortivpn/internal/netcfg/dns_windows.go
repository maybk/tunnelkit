//go:build windows

package netcfg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"tunnelkit/fortivpn"
)

var (
	modDnsapi                 = windows.NewLazySystemDLL("dnsapi.dll")
	procDnsFlushResolverCache = modDnsapi.NewProc("DnsFlushResolverCache")
)

const nrptPolicyBaseKey = `SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`
const nrptRulePrefix = "TunnelKit-"

func applyWindowsDNS(ctx context.Context, luid winipcfg.LUID, plan DNSPlan, journal *Journal) error {
	_ = luid.FlushDNS(windows.AF_INET6)
	if plan.Mode == fortivpn.DNSOff || len(plan.Servers) == 0 || SplitDNSNeedsDomains(plan) {
		_ = luid.FlushDNS(windows.AF_INET)
		_ = clearWindowsNRPT()
		flushWindowsDNSCache(ctx)
		return nil
	}

	if err := luid.SetDNS(windows.AF_INET, plan.Servers, plan.Domains); err != nil {
		return fmt.Errorf("winipcfg SetDNS: %w", err)
	}

	if plan.Mode == fortivpn.DNSSplit && len(plan.Domains) > 0 {
		if err := journal.Record(JournalEntry{
			Kind: KindWindowsNRPT,
			Key:  nrptRulePrefix,
		}); err != nil {
			return err
		}
		if err := writeWindowsNRPT(plan); err != nil {
			return err
		}
	} else {
		_ = clearWindowsNRPT()
	}

	flushWindowsDNSCache(ctx)
	return nil
}

func removeWindowsDNS(ctx context.Context, luid winipcfg.LUID) error {
	var errs []error
	if luid != 0 {
		if err := luid.FlushDNS(windows.AF_INET); err != nil {
			errs = append(errs, err)
		}
		_ = luid.FlushDNS(windows.AF_INET6)
	}
	if err := clearWindowsNRPT(); err != nil {
		errs = append(errs, err)
	}
	flushWindowsDNSCache(ctx)
	return errors.Join(errs...)
}

func writeWindowsNRPT(plan DNSPlan) error {
	if err := clearWindowsNRPT(); err != nil {
		return err
	}

	servers := make([]string, 0, len(plan.Servers))
	for _, s := range plan.Servers {
		if s.IsValid() {
			servers = append(servers, s.String())
		}
	}
	serverList := strings.Join(servers, ";")

	names := make([]string, 0, len(plan.Domains))
	for _, d := range plan.Domains {
		trimmed := strings.TrimSpace(strings.TrimPrefix(d, "."))
		if trimmed != "" {
			names = append(names, "."+trimmed)
		}
	}
	if len(names) == 0 {
		return nil
	}

	subkeyPath := nrptPolicyBaseKey + `\` + nrptRulePrefix + "0"
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, subkeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create NRPT registry key: %w", err)
	}
	defer key.Close()

	if err := key.SetDWordValue("Version", 2); err != nil {
		return err
	}
	if err := key.SetDWordValue("ConfigOptions", 8); err != nil {
		return err
	}
	if err := key.SetStringValue("GenericDNSServers", serverList); err != nil {
		return err
	}
	if err := key.SetStringsValue("Name", names); err != nil {
		return err
	}
	return nil
}

func clearWindowsNRPT() error {
	baseKey, err := registry.OpenKey(registry.LOCAL_MACHINE, nrptPolicyBaseKey, registry.ENUMERATE_SUB_KEYS|registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer baseKey.Close()

	subkeys, err := baseKey.ReadSubKeyNames(-1)
	if err != nil {
		return err
	}
	var errs []error
	for _, name := range subkeys {
		if strings.HasPrefix(name, nrptRulePrefix) {
			if err := registry.DeleteKey(baseKey, name); err != nil && !errors.Is(err, registry.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func flushWindowsDNSCache(_ context.Context) {
	if err := procDnsFlushResolverCache.Find(); err == nil {
		_, _, _ = procDnsFlushResolverCache.Call()
	}
}
