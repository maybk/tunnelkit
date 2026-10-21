package helper

import (
	"context"
	"errors"
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
	mu            sync.Mutex
	client        *ipc.Conn
	lastPhase     fortivpn.Phase
	lastInfo      *fortivpn.TunnelInfo
	lastStats     *fortivpn.Stats
	lastErr       *fortivpn.Error
	pendingPrompt *fortivpn.Prompt
	promptCh      chan promptResponse
	promptTimeout time.Duration
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

func (s *sessionState) SetClient(c *ipc.Conn) (*fortivpn.Event, *fortivpn.Prompt) {
	s.mu.Lock()
	old := s.client
	s.client = c
	snap := s.snapshotLocked()
	var promptCopy *fortivpn.Prompt
	if s.pendingPrompt != nil {
		cp := *s.pendingPrompt
		promptCopy = &cp
	}
	s.mu.Unlock()

	if old != nil && old != c {
		_ = old.Close()
	}
	return snap, promptCopy
}

func (s *sessionState) ClearClient(c *ipc.Conn) {
	s.mu.Lock()
	if s.client == c {
		s.client = nil
	}
	s.mu.Unlock()
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
		_ = client.Send(ipc.TypeEvent, 0, ev)
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
		_ = client.Send(ipc.TypePrompt, 0, p)
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
