//go:build !darwin

package netcfg

func defaultDarwinRouteOps() darwinRouteOps {
	return nil
}
