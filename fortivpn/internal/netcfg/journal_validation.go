package netcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

func decodeJournal(data []byte) ([]JournalEntry, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	if len(data) > 64<<10 {
		return nil, errors.New("network recovery journal is too large")
	}
	var entries []JournalEntry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("unexpected data after network recovery journal")
	}
	if len(entries) > 16 {
		return nil, errors.New("too many network recovery entries")
	}
	validIPv4 := func(value string) bool {
		ip, err := netip.ParseAddr(value)
		return err == nil && ip.Is4() && !ip.IsUnspecified() && !ip.IsMulticast()
	}
	for _, entry := range entries {
		switch entry.Kind {
		case KindHostRoute:
			if !validIPv4(entry.Gateway) || (entry.NextHop != "" && !validIPv4(entry.NextHop)) || (entry.Iface != "" && !journalInterface.MatchString(entry.Iface)) || entry.Key != "" {
				return nil, errors.New("invalid host-route recovery entry")
			}
		case KindDarwinDNSKey:
			if entry.Key != "State:/Network/Service/TunnelKit/DNS" && entry.Key != "State:/Network/Service/TunnelKit/IPv4" {
				return nil, errors.New("invalid DNS recovery key")
			}
		case KindWindowsNRPT:
			if entry.Key != "TunnelKit-" {
				return nil, errors.New("invalid NRPT recovery key")
			}
		case KindLinuxDNS:
			if !journalInterface.MatchString(entry.Iface) || (entry.Key != "resolvectl" && entry.Key != "resolvconf") {
				return nil, errors.New("invalid Linux DNS recovery entry")
			}
		default:
			return nil, errors.New("unsupported network recovery entry")
		}
	}
	return entries, nil
}
