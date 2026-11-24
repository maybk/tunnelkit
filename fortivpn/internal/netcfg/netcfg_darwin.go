//go:build darwin

package netcfg

import "context"

// Apply configures the macOS utun interface, gateway host route (native PF_ROUTE),
// tunnel routes (native PF_ROUTE), and scutil DNS settings, recording persistent
// mutations to journalPath first.
func Apply(ctx context.Context, d Desired, journalPath string) (Applied, error) {
	return applyDarwinWithOps(ctx, d, journalPath, defaultDarwinRouteOps(), defaultCmdRunner)
}

// Recover rolls back any residual host routes or scutil DNS keys recorded in
// journalPath from a previously crashed helper process.
func Recover(ctx context.Context, journalPath string) error {
	return recoverDarwinWithOps(ctx, journalPath, defaultDarwinRouteOps(), defaultCmdRunner)
}
