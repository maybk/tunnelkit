//go:build linux

package netcfg

import "context"

// Apply configures the Linux TUN interface and routes via native Netlink and
// systemd-resolved / resolvconf DNS, recording persistent mutations to journalPath first.
func Apply(ctx context.Context, d Desired, journalPath string) (Applied, error) {
	return applyLinuxWithOps(ctx, d, journalPath, defaultLinuxNetlinkOps(), defaultCmdRunner)
}

// Recover rolls back any residual host routes or DNS configurations recorded in
// journalPath from a previously crashed helper process.
func Recover(ctx context.Context, journalPath string) error {
	return recoverLinuxWithOps(ctx, journalPath, defaultLinuxNetlinkOps(), defaultCmdRunner)
}
