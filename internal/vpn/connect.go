package vpn

import (
	"context"
	"errors"
	"os"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
)

var ErrAlreadyConnected = errors.New("已有活动连接，请先断开后再连接")

// Connect 启动指定配置的 VPN 连接并等待隧道就绪；若已有其他配置处于连接或正在连接状态，会自动先断开再切换。
// 用户取消授权或调用 Cancel 时返回 nil。
func (s *Service) Connect(ctx context.Context, profileID string) error {
	s.mu.RLock()
	inFlightCancel := s.cancelConnect
	inFlightDone := s.opDone
	sameProfile := (s.state.ProfileID == profileID && s.state.Status != StatusDisconnected) ||
		(s.inFlightProfileID == profileID && inFlightCancel != nil)
	s.mu.RUnlock()

	if !sameProfile && inFlightDone != nil {
		if inFlightCancel != nil {
			inFlightCancel()
		}
		select {
		case <-inFlightDone:
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
			return ErrBusy
		}
	}

	release, err := s.begin()
	if err != nil {
		return err
	}
	defer release()

	opCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancelConnect = cancel
	s.inFlightProfileID = profileID
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelConnect = nil
		s.inFlightProfileID = ""
		s.mu.Unlock()
		cancel()
	}()

	if restored, err := s.disconnectIfActive(opCtx, profileID); err != nil {
		return err
	} else if restored {
		return nil
	}
	if opCtx.Err() != nil {
		return nil
	}

	prof, password, err := s.profiles.GetWithPassword(profileID)
	if err != nil {
		s.failConnectErr(profileID, "", &Error{Message: "读取连接配置失败", Hint: err.Error()}, err)
		return err
	}

	cfg := prof.ToConfig(password)
	if err := cfg.Validate(); err != nil {
		_ = s.resetLog(prof.ID)
		var fe *fortivpn.Error
		if errors.As(err, &fe) {
			vpnErr := MapError(fe)
			s.failConnectErr(prof.ID, prof.Name, vpnErr, err)
			return errors.New(vpnErr.Message)
		}
		s.failConnectErr(prof.ID, prof.Name, &Error{Message: err.Error()}, err)
		return err
	}

	if err := s.resetLog(prof.ID); err != nil {
		s.failConnectErr(prof.ID, prof.Name, &Error{Message: "初始化连接日志失败", Hint: err.Error()}, err)
		return err
	}
	s.appendLogf(prof.ID, "INFO", "正在启动 VPN 连接 (配置=%s, 网关=%s:%d)", prof.Name, prof.Host, prof.Port)

	s.setState(State{
		Status:      StatusConnecting,
		Phase:       fortivpn.PhaseConnecting,
		ProfileID:   prof.ID,
		ProfileName: prof.Name,
	})
	cancelled := func() error {
		_ = s.tickets.Clear()
		s.appendLogf(prof.ID, "INFO", "已取消连接请求")
		s.setState(State{
			Status:      StatusDisconnected,
			Phase:       fortivpn.PhaseDisconnected,
			ProfileID:   prof.ID,
			ProfileName: prof.Name,
		})
		return nil
	}

	ticketSaved := false
	conn, ticket, err := s.launcher.Launch(opCtx, helperclient.LaunchRequest{
		Config:      cfg,
		ProfileID:   prof.ID,
		ProfileName: prof.Name,
		OnReady: func(ticket helperclient.Ticket) error {
			if err := s.tickets.Save(ticket); err != nil {
				return err
			}
			ticketSaved = true
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, helperclient.ErrRecoveryPending) {
			s.failConnectErr(prof.ID, prof.Name, errPriorSessionPending(), err)
			return err
		}
		if errors.Is(err, helperclient.ErrElevationCancelled) || opCtx.Err() != nil {
			return cancelled()
		}
		if errors.Is(err, helperclient.ErrHelperMissing) {
			vpnErr := &Error{
				Message: "未找到隧道组件 tunnelkit-helper",
				Hint:    "请确认应用已完整安装；若在开发环境运行，请先执行 task build:helper。",
			}
			s.failConnectErr(prof.ID, prof.Name, vpnErr, err)
			return err
		}
		if errors.Is(err, helperclient.ErrProtocolMismatch) {
			vpnErr := &Error{
				Message: "隧道组件版本不匹配",
				Hint:    "请先断开现有连接或重启应用以更新后台组件。",
			}
			s.failConnectErr(prof.ID, prof.Name, vpnErr, err)
			return err
		}
		s.failConnectErr(prof.ID, prof.Name, &Error{Message: "启动连接失败", Hint: err.Error()}, err)
		return err
	}

	if ticket.ProfileID == "" {
		ticket.ProfileID = prof.ID
	}
	if ticket.ProfileName == "" {
		ticket.ProfileName = prof.Name
	}
	if ticket.StartedAt == "" {
		ticket.StartedAt = time.Now().Format(time.RFC3339)
	}

	if err := func() error {
		if ticketSaved {
			return nil
		}
		return s.tickets.Save(ticket)
	}(); err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = conn.Stop(stopCtx)
		stopCancel()
		_ = conn.Close()
		s.failConnectErr(prof.ID, prof.Name, &Error{Message: "保存连接状态失败，已请求中止连接", Hint: err.Error()}, err)
		return err
	}

	readyCh, streamDone := s.startEventStream(conn, ticket, nil)

	res, timedOut := s.waitReady(opCtx, readyCh)
	if opCtx.Err() == nil && !timedOut {
		if res.err != nil {
			if res.cleanExit {
				waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
				streamErr := s.waitStreamDoneContext(waitCtx, streamDone)
				helperErr := s.waitForHelper(waitCtx)
				waitCancel()
				if streamErr == nil && helperErr == nil {
					_ = s.tickets.Clear()
				}
			}
			return errors.New(res.err.Message)
		}
		return nil
	}
	priorError := s.State().Error
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 12*time.Second)
	err = s.disconnectLocked(stopCtx)
	stopCancel()
	if err != nil {
		return err
	}
	if opCtx.Err() != nil {
		if priorError != nil {
			s.failConnectErr(prof.ID, prof.Name, priorError, nil)
			return errors.New(priorError.Message)
		}
		return cancelled()
	}
	timeoutErr := &Error{Message: "连接超时", Hint: "等待网关建立隧道超时，请检查网关状态或查看连接日志。"}
	s.failConnectErr(prof.ID, prof.Name, timeoutErr, nil)
	return errors.New(timeoutErr.Message)
}

func (s *Service) disconnectIfActive(ctx context.Context, targetProfileID string) (bool, error) {
	s.mu.RLock()
	activeConn := s.conn
	curState := s.state
	s.mu.RUnlock()

	if activeConn != nil {
		if curState.ProfileID == targetProfileID {
			return false, ErrAlreadyConnected
		}
		return false, s.disconnectLocked(ctx)
	}

	if oldTicket, loadErr := s.tickets.Load(); loadErr == nil {
		oldConn, snap, recovered, err := s.attachOrRecoverTicket(ctx, oldTicket)
		if err != nil {
			s.recordAttachOrRecoverFailure(oldTicket, recovered, err)
			return false, err
		}
		if recovered {
			return false, nil
		}
		s.adoptSession(oldConn, snap, oldTicket)
		if oldTicket.ProfileID == targetProfileID {
			return true, nil
		}
		return false, s.disconnectLocked(ctx)
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		s.failDisconnect(targetProfileID, "", errSessionRead())
		return false, loadErr
	}
	if err := s.waitForHelperOrFail(ctx, curState.ProfileID, curState.ProfileName); err != nil {
		return false, err
	}
	if handled, err := s.recoverLegacyNetwork(ctx, helperclient.Ticket{
		ProfileID:   curState.ProfileID,
		ProfileName: curState.ProfileName,
	}); handled {
		return false, err
	}
	return false, nil
}

// failConnectErr 先落盘脱敏诊断日志再发布状态，保证前端收到状态变更回读日志时已包含本次错误记录。
func (s *Service) failConnectErr(profileID, profileName string, vpnErr *Error, cause error) {
	s.appendErrorLog(profileID, vpnErr, cause)
	s.setState(State{
		Status:      StatusDisconnected,
		Phase:       fortivpn.PhaseDisconnected,
		ProfileID:   profileID,
		ProfileName: profileName,
		Error:       vpnErr,
	})
}
