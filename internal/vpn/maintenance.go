package vpn

import (
	"context"
	"time"
)

// BeginUpdate 停止活动会话并恢复网络后进入维护状态；自更新替换二进制期间拒绝新的连接操作。
func (s *Service) BeginUpdate(ctx context.Context) error {
	ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Second)
	defer cancelTimeout()

	if err := s.cancelInFlight(ctx); err != nil {
		return err
	}

	release, err := s.begin()
	if err != nil {
		return err
	}
	defer release()

	s.mu.Lock()
	s.maintenance = true
	s.mu.Unlock()

	if err := s.disconnectLocked(ctx); err != nil {
		s.mu.Lock()
		s.maintenance = false
		s.mu.Unlock()
		return err
	}
	return nil
}

// EndUpdate 在更新失败或取消时恢复操作入口；更新成功准备重启时关闭入口。
func (s *Service) EndUpdate(restarting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if restarting {
		s.closing = true
	}
	s.maintenance = false
}
