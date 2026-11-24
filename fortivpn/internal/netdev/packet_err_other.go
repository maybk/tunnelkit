//go:build !unix && !windows

package netdev

func isPacketRejectedErr(error) bool {
	return false
}
