package helper

import (
	"testing"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

func TestWriterPreservesPhaseChangesUnderLogBackpressure(t *testing.T) {
	s := newSessionState(0)
	s.writer = newEventWriter(nil)
	waiting := outboundFrame{msgType: ipc.TypeEvent, payload: fortivpn.Event{Phase: fortivpn.PhaseWaitingInput}}
	s.enqueueFrameLocked(waiting)
	for i := 1; i < maxQueuedFrames; i++ {
		s.enqueueFrameLocked(outboundFrame{msgType: ipc.TypeLog})
	}
	if !s.enqueueFrameLocked(outboundFrame{msgType: ipc.TypePrompt}) || s.writer.queue[0].msgType != ipc.TypeEvent {
		t.Fatal("log backpressure evicted a phase change")
	}
}

func TestWriterReportsCriticalQueueOverflow(t *testing.T) {
	s := newSessionState(0)
	s.writer = newEventWriter(nil)
	for i := 0; i < maxQueuedFrames; i++ {
		s.enqueueFrameLocked(outboundFrame{msgType: ipc.TypeEvent})
	}
	if s.enqueueFrameLocked(outboundFrame{msgType: ipc.TypePrompt}) || !s.writer.failed {
		t.Fatal("critical queue overflow was hidden")
	}
}
