//go:build !linux

package netcfg

func defaultLinuxNetlinkOps() linuxNetlinkOps {
	return nil
}
