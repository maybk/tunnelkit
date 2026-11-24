//go:build !unix && !windows

package netdev

func samplePacketRejectedErr() error {
	return nil
}
