package helper

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

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
	writer          *eventWriter
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
	oldClient := s.client
	oldWriter := s.writer
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
	if err == nil {
		err = flushLogBacklog(c, s.logBacklog)
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	_ = c.SetWriteDeadline(time.Time{})
	s.logBacklog = nil
	s.logBacklogBytes = 0
	s.client = c
	w := newEventWriter(c)
	s.writer = w
	s.mu.Unlock()

	if oldWriter != nil {
		oldWriter.close()
	}
	if oldClient != nil && oldClient != c {
		_ = oldClient.Close()
	}
	go s.writeEvents(w)
	return nil
}

func (s *sessionState) ClearClient(c *ipc.Conn) bool {
	s.mu.Lock()
	detached := s.client == c
	var w *eventWriter
	if detached {
		s.client = nil
		w = s.writer
		s.writer = nil
	}
	s.mu.Unlock()
	if w != nil {
		w.close()
	}
	return detached
}

func (s *sessionState) CloseClient() {
	s.mu.Lock()
	c := s.client
	w := s.writer
	s.client = nil
	s.writer = nil
	s.mu.Unlock()
	if w != nil {
		w.close()
	}
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
	s.enqueueFrameLocked(outboundFrame{
		msgType: ipc.TypeEvent,
		payload: ev,
	})
	s.mu.Unlock()
}

func (s *sessionState) Prompt(ctx context.Context, p fortivpn.Prompt) (string, error) {
	ch := make(chan promptResponse, 1)

	s.mu.Lock()
	s.pendingPrompt = &p
	s.promptCh = ch
	s.enqueueFrameLocked(outboundFrame{
		msgType: ipc.TypePrompt,
		payload: p,
	})
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.promptCh == ch {
			s.pendingPrompt = nil
			s.promptCh = nil
		}
		s.mu.Unlock()
	}()

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

func (s *sessionState) HasClient() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil
}
