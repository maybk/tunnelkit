//go:build darwin

package netdev

import "testing"

func TestComputeUTUNMaxPendingPackets(t *testing.T) {
	cases := []struct {
		name    string
		recvBuf int
		mtu     int
		wantMin int
		wantMax int
	}{
		{name: "default buffer caps at 64", recvBuf: darwinUtunRecvBufDefault, mtu: 1400, wantMin: 64, wantMax: 64},
		{name: "8MiB buffer caps at 64", recvBuf: darwinUtunRecvBufTarget, mtu: 1500, wantMin: 64, wantMax: 64},
		{name: "small buffer stays under 7/8 recvBuf", recvBuf: 32 << 10, mtu: 1500, wantMin: 18, wantMax: 19},
		{name: "tiny buffer floors at 1", recvBuf: 512, mtu: 1500, wantMin: 1, wantMax: 1},
		{name: "zero values fall back safely", recvBuf: 0, mtu: 0, wantMin: 64, wantMax: 64},
		{name: "jumbo mtu reduces pending count", recvBuf: 128 << 10, mtu: 9000, wantMin: 12, wantMax: 13},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeUTUNMaxPendingPackets(tc.recvBuf, tc.mtu)
			if got < tc.wantMin || got > tc.wantMax {
				t.Fatalf("computeUTUNMaxPendingPackets(%d, %d) = %d, want [%d, %d]", tc.recvBuf, tc.mtu, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestTuneDarwinUTUNNilFileIsSafe(t *testing.T) {
	tuneDarwinUTUN(nil, 1400)
}
