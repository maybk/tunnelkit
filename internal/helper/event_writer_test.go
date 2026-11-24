package helper

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

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
	for range maxQueuedFrames {
		s.enqueueFrameLocked(outboundFrame{msgType: ipc.TypeEvent})
	}
	if s.enqueueFrameLocked(outboundFrame{msgType: ipc.TypePrompt}) || !s.writer.failed {
		t.Fatal("critical queue overflow was hidden")
	}
}

func TestPopFrontAndBoundedLogBacklogPreserveCapacity(t *testing.T) {
	queue := make([]outboundFrame, 0, 8)
	queue = append(queue,
		outboundFrame{msgType: ipc.TypeLog, logLine: "first"},
		outboundFrame{msgType: ipc.TypeLog, logLine: "second"},
	)
	initialCap := cap(queue)

	for i := range 200 {
		got, next := popFront(queue)
		if got.logLine == "" {
			t.Fatalf("iteration %d: unexpected empty dequeued item", i)
		}
		queue = append(next, outboundFrame{msgType: ipc.TypeLog, logLine: "next"})
		if cap(queue) != initialCap {
			t.Fatalf("iteration %d: queue capacity drifted from %d to %d", i, initialCap, cap(queue))
		}
	}
	if backing := queue[:cap(queue)]; backing[len(queue)].logLine != "" || backing[len(queue)].payload != nil {
		t.Fatalf("expected popped tail slot to be zeroed for GC, got %+v", backing[len(queue)])
	}

	s := newSessionState(0)
	chunk := strings.Repeat("a", 8<<10) // 8 KiB per line, cap at 128 KiB (~16 lines)
	for range 20 {
		s.appendLogBacklogLocked(chunk)
	}
	steadyCap := cap(s.logBacklog)
	for range 200 {
		s.appendLogBacklogLocked(chunk)
		if cap(s.logBacklog) != steadyCap {
			t.Fatalf("logBacklog capacity drifted from %d to %d", steadyCap, cap(s.logBacklog))
		}
		if s.logBacklogBytes > 128<<10 {
			t.Fatalf("logBacklogBytes %d exceeded 128 KiB limit", s.logBacklogBytes)
		}
	}
}

func TestFlushEventsWaitsUntilInFlightFrameCompletes(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	s := newSessionState(0)
	s.client = ipc.NewConn(server)
	w := newEventWriter(s.client)
	s.writer = w
	go s.writeEvents(w)
	defer s.CloseClient()

	s.OnEvent(fortivpn.Event{Phase: fortivpn.PhaseDisconnected})

	// With an unread synchronous net.Pipe, FlushEvents must honor context expiry
	// instead of returning before the queued frame is written.
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	s.FlushEvents(shortCtx)
	shortCancel()

	flushed := make(chan struct{})
	go func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer flushCancel()
		s.FlushEvents(flushCtx)
		close(flushed)
	}()

	select {
	case <-flushed:
		t.Fatal("FlushEvents returned before peer read the in-flight frame")
	case <-time.After(20 * time.Millisecond):
	}

	env, err := ipc.NewConn(client).ReadEnvelope()
	if err != nil {
		t.Fatalf("ReadEnvelope: %v", err)
	}
	if env.Type != ipc.TypeEvent {
		t.Fatalf("envelope type = %q, want %q", env.Type, ipc.TypeEvent)
	}

	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("FlushEvents did not unblock after frame write completed")
	}
}
