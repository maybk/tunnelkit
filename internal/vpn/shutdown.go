package vpn

import (
	"context"
	"time"
)

// Shutdown 停止接收新的连接操作，等待进行中的恢复、连接或断开流程收敛后停止当前会话；
// 若清理失败则保留恢复票据，供下次退出或启动重试。
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

// waitForHelper 等待底层 helper 进程退出，同时覆盖退出请求前刚收到终态事件的窗口。
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
