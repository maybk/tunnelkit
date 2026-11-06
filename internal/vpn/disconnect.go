package vpn

import (
	"context"
	"errors"
	"os"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/ipc"
)

// Disconnect 断开当前活动连接（通过已建立的 IPC 通道通知 helper，无需再次弹出管理员授权）。
// 若当前有正在进行的连接操作，会先中止该连接并等待其清理完成。
func (s *Service) Disconnect(ctx context.Context) error {
	ctx, cancelTimeout := context.WithTimeout(ctx, 12*time.Second)
	defer cancelTimeout()
	s.mu.RLock()
	inFlightCancel := s.cancelConnect
	inFlightDone := s.opDone
	s.mu.RUnlock()

	if inFlightDone != nil {
		if inFlightCancel != nil {
			inFlightCancel()
		}
		select {
		case <-inFlightDone:
		case <-ctx.Done():
			return ctx.Err()
		}
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
			if attached, snap, attachErr := s.launcher.Attach(ctx, ticket); attachErr == nil {
				if snap != nil && snap.Phase == fortivpn.PhaseDisconnected {
					return s.finishSession(ctx, attached, snap, ticket)
				}
				conn = attached
				_, streamDone = s.startEventStream(conn, ticket)
				curState = s.State()
			} else {
				if !ipc.EndpointUnavailable(attachErr) {
					return attachErr
				}
				return s.recoverNetwork(ctx, ticket)
			}
		} else if !errors.Is(loadErr, os.ErrNotExist) {
			return loadErr
		}
	}

	if conn == nil {
		if err := s.waitForHelper(ctx); err != nil {
			return err
		}
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

	if err := s.waitStreamDoneContext(ctx, streamDone); err != nil {
		return err
	}
	if err := s.waitForHelper(ctx); err != nil {
		return err
	}
	if final := s.State(); final.Error != nil {
		return errors.New(final.Error.Message)
	}
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
