package netcfg

import (
	"errors"
	"net/netip"
	"regexp"
)

var journalInterface = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,64}$`)

// ValidateJournalData restricts recovery to the resource types emitted by this
// package. Journals are data, never shell fragments or arbitrary system paths.
func ValidateJournalData(data []byte) error {
	_, err := decodeJournal(data)
	return err
}

func validateJournalEntries(entries []JournalEntry) error {
	if len(entries) > 16 {
		return errors.New("too many network recovery entries")
	}
	validIPv4 := func(value string) bool {
		ip, err := netip.ParseAddr(value)
		return err == nil && ip.Is4() && !ip.IsUnspecified() && !ip.IsMulticast()
	}
	for _, entry := range entries {
		switch entry.Kind {
		case KindHostRoute:
			if !validIPv4(entry.Gateway) || (entry.NextHop != "" && !validIPv4(entry.NextHop)) || (entry.Iface != "" && !journalInterface.MatchString(entry.Iface)) || entry.Key != "" {
				return errors.New("invalid host-route recovery entry")
			}
		case KindDarwinDNSKey:
			if entry.Key != "State:/Network/Service/TunnelKit/DNS" && entry.Key != "State:/Network/Service/TunnelKit/IPv4" {
				return errors.New("invalid DNS recovery key")
			}
		case KindWindowsNRPT:
			if entry.Key != "TunnelKit-" {
				return errors.New("invalid NRPT recovery key")
			}
		case KindLinuxDNS:
			if !journalInterface.MatchString(entry.Iface) || (entry.Key != "resolvectl" && entry.Key != "resolvconf") {
				return errors.New("invalid Linux DNS recovery entry")
			}
		default:
			return errors.New("unsupported network recovery entry")
		}
	}
	return nil
}
