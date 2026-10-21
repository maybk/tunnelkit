//go:build linux

package netcfg

import "context"

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
