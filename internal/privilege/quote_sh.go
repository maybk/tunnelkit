package privilege

import "strings"

// ShellQuote wraps value in single quotes and escapes embedded single quotes for POSIX shells.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// ShellQuoteCommand builds a space-separated POSIX shell command with each argument safely quoted.
func ShellQuoteCommand(bin string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, ShellQuote(bin))
	for _, a := range args {
		parts = append(parts, ShellQuote(a))
	}
	return strings.Join(parts, " ")
}
