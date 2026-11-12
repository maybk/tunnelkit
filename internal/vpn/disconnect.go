package vpn

import (
	"context"
	"errors"
	"os"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
)

// Disconnect 断开当前活动连接（通过已建立的 IPC 通道通知 helper，无需再次弹出管理员授权）。
// 若当前有正在进行的连接操作，会先中止该连接并等待其清理完成。
func (s *Service) Disconnect(ctx context.Context) error {
	ctx, cancelTimeout := context.WithTimeout(ctx, 12*time.Second)
	defer cancelTimeout()
	if err := s.cancelInFlight(ctx); err != nil {
		return err
	}

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
			attached, _, recovered, err := s.attachOrRecoverTicket(ctx, ticket)
			if err != nil {
				s.recordAttachOrRecoverFailure(ticket, recovered, err)
				return err
			}
			if recovered {
				return nil
			}
			conn = attached
			_, streamDone = s.startEventStream(conn, ticket, nil)
			curState = s.State()
		} else if !errors.Is(loadErr, os.ErrNotExist) {
			s.failDisconnect(curState.ProfileID, curState.ProfileName, errSessionRead())
			return loadErr
		}
	}

	if conn == nil {
		if err := s.waitForHelperOrFail(ctx, curState.ProfileID, curState.ProfileName); err != nil {
			return err
		}
		if handled, err := s.recoverLegacyNetwork(ctx, helperclient.Ticket{
			ProfileID:   curState.ProfileID,
			ProfileName: curState.ProfileName,
		}); handled {
			return err
		}
		if err := s.tickets.Clear(); err != nil {
			return err
		}
		s.setState(State{
			Status:      StatusDisconnected,
			Phase:       fortivpn.PhaseDisconnected,
			ProfileID:   curState.ProfileID,
			ProfileName: curState.ProfileName,
		})
		return nil
	}

	s.updateState(conn, func(state *State) bool {
		state.Status = StatusDisconnecting
		state.Phase = fortivpn.PhaseDisconnecting
		state.Prompt = nil
		state.Error = nil
		return true
	})

	stopCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	err := conn.Stop(stopCtx)
	cancel()
	if err != nil {
		// 不能以早先快照恢复状态：停止请求失败并不证明隧道仍连接或旧挑战仍有效；
		// 解除当前控制连接并收敛至断开异常态，保留恢复票据供后续重试清理。
		s.failDisconnect(curState.ProfileID, curState.ProfileName, errDisconnectStop())
		return err
	}

	if err := s.waitStreamDoneContext(ctx, streamDone); err != nil {
		s.failDisconnect(curState.ProfileID, curState.ProfileName, errDisconnectTimeout())
		return err
	}
	if err := s.waitForHelperOrFail(ctx, curState.ProfileID, curState.ProfileName); err != nil {
		return err
	}
	if final := s.State(); final.Error != nil {
		return errors.New(final.Error.Message)
	}
	if ticket, err := s.tickets.Load(); err == nil {
		// 旧协议的无错误终态不保证网络已恢复，必须检查遗留记录后再清除票据。
		if ticket.Protocol == 1 {
			return s.finishSession(ctx, nil, &fortivpn.Event{Phase: fortivpn.PhaseDisconnected}, ticket)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		s.failDisconnect(curState.ProfileID, curState.ProfileName, errSessionRead())
		return err
	}
	s.detachConn()
	if err := s.tickets.Clear(); err != nil {
		return err
	}

	s.setState(State{
		Status:      StatusDisconnected,
		Phase:       fortivpn.PhaseDisconnected,
		ProfileID:   curState.ProfileID,
		ProfileName: curState.ProfileName,
	})
	return nil
}

func (s *Service) failDisconnect(profileID, profileName string, vpnErr *Error) {
	s.detachConn()
	s.mu.RLock()
	targetProfileID := profileID
	if targetProfileID == "" {
		targetProfileID = s.state.ProfileID
	}
	targetErr := vpnErr
	if targetErr == nil {
		targetErr = s.state.Error
	}
	s.mu.RUnlock()
	if targetProfileID != "" && targetErr != nil {
		s.appendErrorLog(targetProfileID, targetErr, nil)
	}
	s.updateState(nil, func(state *State) bool {
		if profileID == "" {
			profileID = state.ProfileID
		}
		if profileName == "" {
			profileName = state.ProfileName
		}
		if vpnErr == nil && state.Error != nil {
			vpnErr = state.Error
		}
		*state = State{
			Status:      StatusDisconnected,
			Phase:       fortivpn.PhaseDisconnected,
			ProfileID:   profileID,
			ProfileName: profileName,
			Error:       vpnErr,
		}
		return true
	})
}
