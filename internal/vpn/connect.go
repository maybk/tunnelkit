package vpn

import (
	"context"
	"errors"
	"runtime"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/storage"
)

var ErrAlreadyConnected = errors.New("已有活动连接，请先断开后再连接")

// Connect 启动指定配置的 VPN 连接并等待隧道就绪；若已有其他配置处于连接或正在连接状态，会自动先断开再切换。
// 用户取消授权或调用 Cancel 时返回 nil。
func (s *Service) Connect(ctx context.Context, profileID string) error {
	s.mu.RLock()
	inFlightCancel := s.cancelConnect
	inFlightDone := s.opDone
	sameProfile := s.state.ProfileID == profileID && s.state.Status != StatusDisconnected
	s.mu.RUnlock()

	if !sameProfile && inFlightCancel != nil && inFlightDone != nil {
		inFlightCancel()
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
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancelConnect = nil
		s.mu.Unlock()
		cancel()
	}()

	if err := s.disconnectIfActive(opCtx, profileID); err != nil {
		return err
	}
	if opCtx.Err() != nil {
		return nil
	}

	prof, password, err := s.profiles.GetWithPassword(profileID)
	if err != nil {
		s.failConnect(profileID, "", &Error{Message: "读取连接配置失败", Hint: err.Error()})
		return err
	}

	cfg := prof.ToConfig(password)
	if err := cfg.Validate(); err != nil {
		var fe *fortivpn.Error
		if errors.As(err, &fe) {
			vpnErr := MapError(fe)
			s.failConnect(prof.ID, prof.Name, vpnErr)
			return errors.New(vpnErr.Message)
		}
		s.failConnect(prof.ID, prof.Name, &Error{Message: err.Error()})
		return err
	}

	logPath := storage.LogPath(s.rootDir, prof.ID)
	if err := storage.ResetFile(logPath); err != nil {
		s.failConnect(prof.ID, prof.Name, &Error{Message: "初始化连接日志失败", Hint: err.Error()})
		return err
	}

	s.setState(State{
		Status:      StatusConnecting,
		Phase:       fortivpn.PhaseConnecting,
		ProfileID:   prof.ID,
		ProfileName: prof.Name,
	})
	cancelled := func() error {
		_ = s.tickets.Clear()
		s.setState(State{
			Status:      StatusDisconnected,
			Phase:       fortivpn.PhaseDisconnected,
			ProfileID:   prof.ID,
			ProfileName: prof.Name,
		})
		return nil
	}

	var endpoint string
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		endpoint = storage.HelperSocketPath(s.rootDir)
	}

	conn, ticket, err := s.launcher.Launch(opCtx, helperclient.LaunchRequest{
		Endpoint:    endpoint,
		LogPath:     logPath,
		JournalPath: storage.JournalPath(s.rootDir),
		Config:      cfg,
		ProfileID:   prof.ID,
		ProfileName: prof.Name,
	})
	if err != nil {
		if errors.Is(err, helperclient.ErrElevationCancelled) || opCtx.Err() != nil {
			return cancelled()
		}
		if errors.Is(err, helperclient.ErrHelperMissing) {
			vpnErr := &Error{
				Message: "未找到隧道组件 tunnelkit-helper",
				Hint:    "请确认应用已完整安装；若在开发环境运行，请先执行 task build:helper。",
			}
			s.failConnect(prof.ID, prof.Name, vpnErr)
			return err
		}
		if errors.Is(err, helperclient.ErrProtocolMismatch) {
			vpnErr := &Error{
				Message: "隧道组件版本不匹配",
				Hint:    "请先断开现有连接或重启应用以更新后台组件。",
			}
			s.failConnect(prof.ID, prof.Name, vpnErr)
			return err
		}
		s.failConnect(prof.ID, prof.Name, &Error{Message: "启动连接失败", Hint: err.Error()})
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

	if err := s.tickets.Save(ticket); err != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = conn.Stop(stopCtx)
		stopCancel()
		_ = conn.Close()
		s.failConnect(prof.ID, prof.Name, &Error{Message: "保存连接状态失败，已中止连接", Hint: err.Error()})
		return err
	}

	readyCh, streamDone := s.startEventStream(conn, ticket)

	timer := time.NewTimer(s.readyTimeout)
	defer timer.Stop()

	select {
	case vpnErr := <-readyCh:
		if vpnErr != nil {
			if opCtx.Err() != nil {
				return cancelled()
			}
			return errors.New(vpnErr.Message)
		}
		return nil

	case <-opCtx.Done():
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = conn.Stop(stopCtx)
		stopCancel()
		s.waitStreamDone(streamDone, 2*time.Second)
		s.detachConn()
		return cancelled()

	case <-timer.C:
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = conn.Stop(stopCtx)
		stopCancel()
		s.waitStreamDone(streamDone, 2*time.Second)
		s.detachConn()
		_ = s.tickets.Clear()
		timeoutErr := &Error{
			Message: "连接超时",
			Hint:    "等待网关建立隧道超时，请检查网关状态或查看连接日志。",
		}
		s.failConnect(prof.ID, prof.Name, timeoutErr)
		return errors.New(timeoutErr.Message)
	}
}

func (s *Service) disconnectIfActive(ctx context.Context, targetProfileID string) error {
	s.mu.RLock()
	activeConn := s.conn
	curState := s.state
	s.mu.RUnlock()

	if activeConn != nil {
		if curState.ProfileID == targetProfileID {
			return ErrAlreadyConnected
		}
		return s.disconnectLocked(ctx)
	}

	if oldTicket, loadErr := s.tickets.Load(); loadErr == nil {
		if oldConn, snap, attachErr := s.launcher.Attach(ctx, oldTicket); attachErr == nil {
			if snap != nil && snap.Phase == fortivpn.PhaseDisconnected {
				_ = oldConn.Close()
				_ = s.tickets.Clear()
				return nil
			}
			if oldTicket.ProfileID == targetProfileID {
				_ = oldConn.Close()
				return ErrAlreadyConnected
			}
			_, _ = s.startEventStream(oldConn, oldTicket)
			return s.disconnectLocked(ctx)
		}
		_ = s.tickets.Clear()
	}
	return nil
}

func (s *Service) failConnect(profileID, profileName string, vpnErr *Error) {
	s.setState(State{
		Status:      StatusDisconnected,
		Phase:       fortivpn.PhaseDisconnected,
		ProfileID:   profileID,
		ProfileName: profileName,
		Error:       vpnErr,
	})
}
