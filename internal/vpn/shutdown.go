package vpn

import (
	"context"
	"time"
)

// Shutdown stops accepting new operations, waits for any recovery/connection/
// disconnection already in progress, then stops the remaining session.
// A failed cleanup keeps its ticket so another Shutdown or app launch can retry.
func (s *Service) Shutdown(ctx context.Context) (err error) {
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()
	select {
	case s.shutdownGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.shutdownGate }()
	defer func() {
		if err != nil {
			s.mu.Lock()
			s.closing = false
			s.mu.Unlock()
			state := s.State()
			state.Error = &Error{Message: "清理 VPN 会话失败，请稍后重试断开或退出", Hint: err.Error()}
			s.setState(state)
		}
	}()
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return nil
	}
	s.closing = true
	done, cancel := s.opDone, s.cancelConnect
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.disconnectLocked(ctx)
}

func (s *Service) waitStreamDoneContext(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitForHelper also covers a terminal event received just before the quit request.
func (s *Service) waitForHelper(ctx context.Context) error {
	s.mu.RLock()
	wait := s.waitHelper
	s.mu.RUnlock()
	if wait == nil {
		return nil
	}
	if err := wait(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	s.waitHelper = nil
	s.mu.Unlock()
	return nil
}
