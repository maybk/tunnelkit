package vpn

import (
	"context"
	"time"

	"tunnelkit/fortivpn"
)

// Cancel 中止连接请求，或停止接管后/自动重连中正在等待认证的会话。
func (s *Service) Cancel() error {
	s.mu.RLock()
	cancel, conn, status := s.cancelConnect, s.conn, s.state.Status
	s.mu.RUnlock()
	if cancel != nil {
		cancel()
		return nil
	}
	if conn == nil || status != StatusConnecting {
		return nil
	}
	release, err := s.begin()
	if err != nil {
		return err
	}
	defer release()
	s.mu.RLock()
	current := s.conn == conn && s.state.Status == StatusConnecting
	s.mu.RUnlock()
	if !current {
		return nil
	}
	ctx, stop := context.WithTimeout(context.Background(), 12*time.Second)
	defer stop()
	return s.disconnectLocked(ctx)
}

// Answer 只操作指定的当前挑战；过期取消为幂等空操作，不影响后来建立的会话。
func (s *Service) Answer(ctx context.Context, promptID, value string, cancel bool) error {
	s.mu.RLock()
	state, conn := s.state, s.conn
	var cancelConnect context.CancelFunc
	if s.inFlightProfileID == state.ProfileID {
		cancelConnect = s.cancelConnect
	}
	s.mu.RUnlock()
	if conn == nil || state.Phase != fortivpn.PhaseWaitingInput || state.Prompt == nil || promptID == "" || state.Prompt.ID != promptID {
		if cancel {
			return nil
		}
		return ErrNoActivePrompt
	}
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if !cancel {
		return conn.Answer(ctx, promptID, value, false)
	}
	// “取消连接”终止已校验归属的会话，即使 helper 已接收回答、桌面阶段尚未更新。
	if err := conn.Stop(ctx); err != nil {
		return err
	}
	if cancelConnect != nil {
		// 捕获原连接操作的取消函数，不能在 IPC 返回后取消新的全局连接操作。
		cancelConnect()
	}
	return nil
}
