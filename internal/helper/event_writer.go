package helper

import (
	"context"
	"slices"
	"sync"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

const maxQueuedFrames = 128

type outboundFrame struct {
	msgType ipc.MessageType
	payload any
	logLine string
}

// eventWriter 的队列由 sessionState.mu 保护；引擎回调与日志写入只入队，IPC 写入在专用协程完成。
type eventWriter struct {
	client *ipc.Conn
	wake   chan struct{}
	done   chan struct{}
	once   sync.Once
	queue  []outboundFrame
	failed bool
}

func newEventWriter(client *ipc.Conn) *eventWriter {
	return &eventWriter{
		client: client,
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
}

func (w *eventWriter) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *eventWriter) close() {
	w.once.Do(func() { close(w.done) })
}

func popFront[T any](slice []T) (T, []T) {
	item := slice[0]
	return item, slices.Delete(slice, 0, 1)
}

func isDroppableStatsEvent(f outboundFrame) bool {
	if f.msgType != ipc.TypeEvent {
		return false
	}
	ev, ok := f.payload.(fortivpn.Event)
	if !ok {
		return false
	}
	return ev.Phase == fortivpn.PhaseConnected && ev.Stats != nil && ev.Info == nil && ev.Err == nil && ev.Prompt == nil
}

func (s *sessionState) enqueueFrameLocked(f outboundFrame) bool {
	w := s.writer
	if w == nil {
		return false
	}
	if w.failed {
		return false
	}
	if len(w.queue) >= maxQueuedFrames {
		if isDroppableStatsEvent(f) {
			return true
		}
		// 优先淘汰可替代的统计与日志；关键消息满队列时断开控制流，避免静默失序。
		dropped := false
		for i, item := range w.queue {
			if isDroppableStatsEvent(item) || item.msgType == ipc.TypeLog {
				w.queue = slices.Delete(w.queue, i, i+1)
				dropped = true
				break
			}
		}
		if !dropped {
			if f.msgType == ipc.TypeLog {
				return false
			}
			w.failed = true
			w.signal()
			return false
		}
	}
	w.queue = append(w.queue, f)
	w.signal()
	return true
}

func (s *sessionState) writeEvents(w *eventWriter) {
	for {
		select {
		case <-w.done:
			return
		case <-w.wake:
		}
		for {
			s.mu.Lock()
			if s.writer != w {
				s.mu.Unlock()
				return
			}
			if w.failed {
				s.mu.Unlock()
				s.ClearClient(w.client)
				_ = w.client.Close()
				return
			}
			if len(w.queue) == 0 {
				s.mu.Unlock()
				break
			}
			frame, nextQueue := popFront(w.queue)
			w.queue = nextQueue
			s.mu.Unlock()

			deadline := 5 * time.Second
			if frame.msgType == ipc.TypeLog {
				deadline = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			err := w.client.SendContext(ctx, frame.msgType, 0, frame.payload)
			cancel()
			if err != nil {
				if frame.logLine != "" {
					s.mu.Lock()
					s.appendLogBacklogLocked(frame.logLine)
					s.mu.Unlock()
				}
				s.ClearClient(w.client)
				_ = w.client.Close()
				return
			}
		}
	}
}
