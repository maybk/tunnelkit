package helper

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/ipc"
)

type promptResponse struct {
	value  string
	cancel bool
}

type sessionState struct {
	mu              sync.Mutex
	client          *ipc.Conn
	logBacklog      []string
	logBacklogBytes int
	lastPhase       fortivpn.Phase
	lastInfo        *fortivpn.TunnelInfo
	lastStats       *fortivpn.Stats
	lastErr         *fortivpn.Error
	pendingPrompt   *fortivpn.Prompt
	promptCh        chan promptResponse
	promptTimeout   time.Duration
}

func newSessionState(promptTimeout time.Duration) *sessionState {
	if promptTimeout <= 0 {
		promptTimeout = 3 * time.Minute
	}
	return &sessionState{
		lastPhase:     fortivpn.PhaseConnecting,
		promptTimeout: promptTimeout,
	}
}

func (s *sessionState) AttachClient(c *ipc.Conn, helloID uint64, version string) error {
	s.mu.Lock()
	old := s.client
	snap := s.snapshotLocked()
	var promptCopy *fortivpn.Prompt
	if s.pendingPrompt != nil {
		cp := *s.pendingPrompt
		promptCopy = &cp
	}

	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := c.Send(ipc.TypeWelcome, helloID, ipc.WelcomeData{
		Protocol: ipc.ProtocolVersion,
		Version:  version,
		PID:      os.Getpid(),
		Snapshot: snap,
	})
	if err == nil && promptCopy != nil {
		err = c.Send(ipc.TypePrompt, 0, *promptCopy)
	}
	if err == nil && len(s.logBacklog) > 0 {
		backlog := strings.Join(s.logBacklog, "")
		for len(backlog) > 0 && err == nil {
			size := min(16<<10, len(backlog))
			for size > 0 && !utf8.ValidString(backlog[:size]) {
				size--
			}
			if size == 0 {
				break
			}
			err = c.Send(ipc.TypeLog, 0, ipc.LogData{Line: backlog[:size]})
			backlog = backlog[size:]
		}
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	_ = c.SetWriteDeadline(time.Time{})
	s.logBacklog = nil
	s.logBacklogBytes = 0
	s.client = c
	s.mu.Unlock()

	if old != nil && old != c {
		_ = old.Close()
	}
	return nil
}

func (s *sessionState) ClearClient(c *ipc.Conn) bool {
	s.mu.Lock()
	detached := s.client == c
	if detached {
		s.client = nil
	}
	s.mu.Unlock()
	return detached
}

func (s *sessionState) CloseClient() {
	s.mu.Lock()
	c := s.client
	s.client = nil
	s.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

func (s *sessionState) OnEvent(ev fortivpn.Event) {
	s.mu.Lock()
	s.lastPhase = ev.Phase
	if ev.Info != nil {
		cp := *ev.Info
		s.lastInfo = &cp
	}
	if ev.Stats != nil {
		cp := *ev.Stats
		s.lastStats = &cp
	}
	if ev.Err != nil {
		cp := *ev.Err
		s.lastErr = &cp
	}
	if ev.Phase == fortivpn.PhaseDisconnected {
		s.pendingPrompt = nil
	}
	client := s.client
	s.mu.Unlock()

	if client != nil {
		_ = client.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = client.Send(ipc.TypeEvent, 0, ev)
		_ = client.SetWriteDeadline(time.Time{})
	}
}

func (s *sessionState) Prompt(ctx context.Context, p fortivpn.Prompt) (string, error) {
	ch := make(chan promptResponse, 1)

	s.mu.Lock()
	s.pendingPrompt = &p
	s.promptCh = ch
	client := s.client
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.promptCh == ch {
			s.pendingPrompt = nil
			s.promptCh = nil
		}
		s.mu.Unlock()
	}()

	if client != nil {
		_ = client.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = client.Send(ipc.TypePrompt, 0, p)
		_ = client.SetWriteDeadline(time.Time{})
	}

	timer := time.NewTimer(s.promptTimeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		if resp.cancel {
			return "", errors.New("prompt cancelled by user")
		}
		return resp.value, nil
	case <-timer.C:
		return "", context.DeadlineExceeded
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *sessionState) DeliverAnswer(promptID, value string, cancel bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingPrompt == nil || s.promptCh == nil {
		return errors.New("no active prompt")
	}
	if promptID != "" && s.pendingPrompt.ID != promptID {
		return errors.New("prompt ID mismatch")
	}
	select {
	case s.promptCh <- promptResponse{value: value, cancel: cancel}:
		return nil
	default:
		return errors.New("prompt already answered")
	}
}

func (s *sessionState) snapshotLocked() *fortivpn.Event {
	ev := &fortivpn.Event{
		Phase: s.lastPhase,
	}
	if s.lastInfo != nil {
		cp := *s.lastInfo
		ev.Info = &cp
	}
	if s.lastStats != nil {
		cp := *s.lastStats
		ev.Stats = &cp
	}
	if s.lastErr != nil {
		cp := *s.lastErr
		ev.Err = &cp
	}
	if s.pendingPrompt != nil {
		cp := *s.pendingPrompt
		ev.Prompt = &cp
	}
	return ev
}

// sessionLogWriter sends only redacted output from NewLogger. Files are written
// by the ordinary desktop process; the privileged helper never accepts log paths.
type sessionLogWriter struct{ state *sessionState }

func (w sessionLogWriter) Write(p []byte) (int, error) {
	originalSize := len(p)
	line := strings.ToValidUTF8(string(p), "�")
	if len(line) > 16<<10 {
		size := 16 << 10
		for !utf8.ValidString(line[:size]) {
			size--
		}
		line = line[:size]
	}
	w.state.mu.Lock()
	defer w.state.mu.Unlock()
	client := w.state.client
	if client != nil {
		_ = client.SetWriteDeadline(time.Now().Add(2 * time.Second))
		err := client.Send(ipc.TypeLog, 0, ipc.LogData{Line: line})
		_ = client.SetWriteDeadline(time.Time{})
		if err == nil {
			return originalSize, nil
		}
		_ = client.Close()
	}
	// Keep a bounded tail across temporary GUI disconnections without privileged
	// filesystem writes. Attach flushes it as a small number of bounded frames.
	w.state.logBacklog = append(w.state.logBacklog, line)
	w.state.logBacklogBytes += len(line)
	for w.state.logBacklogBytes > 128<<10 {
		w.state.logBacklogBytes -= len(w.state.logBacklog[0])
		w.state.logBacklog[0] = ""
		w.state.logBacklog = w.state.logBacklog[1:]
	}
	return originalSize, nil
}

func (s *sessionState) HasClient() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.client != nil }
