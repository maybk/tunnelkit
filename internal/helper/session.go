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
		promptTimeout = fortivpn.PromptTimeout
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
	defer s.mu.Unlock()
	if ev.Phase == fortivpn.PhaseConnected && ev.Stats != nil && ev.Info == nil && s.lastPhase != fortivpn.PhaseConnected {
		return
	}
	s.lastPhase = ev.Phase
	if ev.Phase != fortivpn.PhaseConnected {
		s.lastInfo, s.lastStats = nil, nil
	}
	if ev.Info != nil && ev.Phase == fortivpn.PhaseConnected {
		cp := *ev.Info
		s.lastInfo = &cp
	}
	if ev.Stats != nil && ev.Phase == fortivpn.PhaseConnected {
		cp := *ev.Stats
		s.lastStats = &cp
	}
	if ev.Err != nil {
		cp := *ev.Err
		s.lastErr = &cp
	} else {
		s.lastErr = nil
	}
	if ev.Phase != fortivpn.PhaseWaitingInput {
		s.revokePromptLocked()
	}
	// Prompt 注册应答通道后才发布交互事件，避免前端先回答、helper 尚不可回答。
	if ev.Phase == fortivpn.PhaseWaitingInput && ev.Prompt != nil {
		return
	}
	s.enqueueFrameLocked(outboundFrame{
		msgType: ipc.TypeEvent,
		payload: ev,
	})
}

func (s *sessionState) Prompt(ctx context.Context, p fortivpn.Prompt) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !p.Valid() {
		return "", errors.New("invalid authentication prompt")
	}
	ch := make(chan promptResponse, 1)

	s.mu.Lock()
	s.revokePromptLocked()
	s.lastPhase = fortivpn.PhaseWaitingInput
	s.pendingPrompt = &p
	s.promptCh = ch
	s.enqueueFrameLocked(outboundFrame{
		msgType: ipc.TypeEvent,
		payload: fortivpn.Event{Phase: fortivpn.PhaseWaitingInput, Prompt: &p},
	})
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
			return "", context.Canceled
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
	if promptID == "" || s.pendingPrompt.ID != promptID {
		return errors.New("prompt ID mismatch")
	}
	select {
	case s.promptCh <- promptResponse{value: value, cancel: cancel}:
		s.pendingPrompt, s.promptCh = nil, nil
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
	if s.lastPhase == fortivpn.PhaseWaitingInput && s.pendingPrompt != nil {
		cp := *s.pendingPrompt
		ev.Prompt = &cp
	}
	return ev
}

func (s *sessionState) revokePromptLocked() {
	if s.promptCh != nil {
		select {
		case s.promptCh <- promptResponse{cancel: true}:
		default:
		}
	}
	s.pendingPrompt, s.promptCh = nil, nil
}

func (s *sessionState) HasClient() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil
}
