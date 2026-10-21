package vpn

import (
	"context"
	"errors"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
)

// Disconnect 断开当前活动连接（通过已建立的 IPC 通道通知 helper，无需再次弹出管理员授权）。
func (s *Service) Disconnect(ctx context.Context) error {
	release, err := s.begin()
	if err != nil {
		return err
	}
	defer release()

	return s.disconnectLocked(ctx)
}

func (s *Service) disconnectLocked(ctx context.Context) error {
	s.mu.RLock()
	conn := s.conn
	streamDone := s.streamDone
	curState := s.state
	s.mu.RUnlock()

	if conn == nil {
		if ticket, loadErr := s.tickets.Load(); loadErr == nil {
			if attached, _, attachErr := s.launcher.Attach(ctx, ticket); attachErr == nil {
				conn = attached
				_, streamDone = s.startEventStream(conn, ticket)
				curState = s.State()
			} else {
				_ = s.tickets.Clear()
			}
		}
	}

	if conn == nil {
		_ = s.tickets.Clear()
		s.setState(State{
			Status:      StatusDisconnected,
			Phase:       fortivpn.PhaseDisconnected,
			ProfileID:   curState.ProfileID,
			ProfileName: curState.ProfileName,
		})
		return nil
	}

	disconnecting := curState
	disconnecting.Status = StatusDisconnecting
	disconnecting.Phase = fortivpn.PhaseDisconnecting
	disconnecting.Prompt = nil
	disconnecting.Error = nil
	s.setState(disconnecting)

	stopCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	err := conn.Stop(stopCtx)
	cancel()
	if err != nil {
		if errors.Is(err, helperclient.ErrElevationCancelled) {
			curState.Error = nil
			s.setState(curState)
			return nil
		}
		curState.Error = &Error{Message: "断开连接失败", Hint: err.Error()}
		s.setState(curState)
		return err
	}

	s.waitStreamDone(streamDone, 5*time.Second)
	s.detachConn()
	_ = s.tickets.Clear()

	s.setState(State{
		Status:      StatusDisconnected,
		Phase:       fortivpn.PhaseDisconnected,
		ProfileID:   curState.ProfileID,
		ProfileName: curState.ProfileName,
	})
	return nil
}
