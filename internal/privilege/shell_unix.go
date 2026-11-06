//go:build darwin || linux

package privilege

import "strings"

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func shellQuoteCommand(bin string, args []string) string {
	parts := []string{shellQuote(bin)}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}
