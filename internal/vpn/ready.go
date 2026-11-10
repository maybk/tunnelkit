package vpn

import (
	"context"
	"time"

	"tunnelkit/fortivpn"
)

// waitReady 将人工验证等待与网络就绪超时分开，同时保留整体上限。
func (s *Service) waitReady(ctx context.Context, ready <-chan *Error) (*Error, bool) {
	network := time.NewTimer(s.readyTimeout)
	defer network.Stop()
	overall := time.NewTimer(s.readyTimeout + fortivpn.PromptTimeout)
	defer overall.Stop()
	wasWaiting := false
	for {
		s.mu.RLock()
		waiting := s.state.Phase == fortivpn.PhaseWaitingInput
		changed := s.stateChanged
		s.mu.RUnlock()
		if waiting && !wasWaiting {
			network.Stop()
		} else if !waiting && wasWaiting {
			network.Reset(s.readyTimeout)
		}
		wasWaiting = waiting
		var timeout <-chan time.Time
		if !waiting {
			timeout = network.C
		}
		select {
		case result := <-ready:
			return result, false
		case <-ctx.Done():
			return nil, false
		case <-changed:
		case <-timeout:
			// 到期与阶段通知同时就绪时，优先核对当前是否已开始等待输入。
			s.mu.RLock()
			input := s.state.Phase == fortivpn.PhaseWaitingInput
			s.mu.RUnlock()
			if !input {
				return nil, true
			}
		case <-overall.C:
			return nil, true
		}
	}
}
