package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"tunnelkit/fortivpn/fortitest"
	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

func TestDeviceReaderPreservesIPv4Packets(t *testing.T) {
	for _, size := range []int{1400, 3000, 9000, 16384, sslvpn.MaxFramePayload - ppp.HeaderLen} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dev := fortitest.NewMemDevice("test", size)
			s := &session{}
			l := &link{dataCh: make(chan *[]byte, 1), doneCh: make(chan struct{})}
			s.activeLink.Store(l)
			done := make(chan struct{})
			go func() {
				defer close(done)
				s.deviceReaderLoop(dev)
			}()
			t.Cleanup(func() { _ = dev.Close(); <-done })

			packet := bytes.Repeat([]byte{0xa5}, size)
			packet[0] = 0x45
			binary.BigEndian.PutUint16(packet[2:4], uint16(size))
			copy(packet[16:20], []byte{10, 0, 0, 1})
			if err := dev.SendFromHost(packet); err != nil {
				t.Fatal(err)
			}
			select {
			case buf := <-l.dataCh:
				defer putDataFrameBuf(buf)
				frame, rest, err := sslvpn.ParseFrame((*buf)[outboundFrameOffset:])
				if err != nil || len(rest) != 0 {
					t.Fatalf("invalid tunnel frame: %v", err)
				}
				got, ok := ppp.ExtractIPv4(frame)
				if !ok || !bytes.Equal(got, packet) {
					t.Fatal("outbound packet changed or was truncated")
				}
			case <-time.After(time.Second):
				t.Fatal("outbound packet did not reach writer")
			}
		})
	}
}
