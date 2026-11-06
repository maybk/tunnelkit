package vpn

import (
	"context"
	"errors"
	"os"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/ipc"
)

// Recover 在应用启动时通过 run/session.json 重新接入正在运行的 helper，或静默清理失效会话。
func (s *Service) Recover(ctx context.Context) State {
	release, err := s.begin()
	if err != nil {
		return s.State()
	}
	defer release()

	ticket, err := s.tickets.Load()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return s.setState(State{
				Status: StatusDisconnected,
				Phase:  fortivpn.PhaseDisconnected,
				Error: &Error{
					Message: "读取会话恢复信息失败",
					Hint:    err.Error(),
				},
			})
		}
		return s.setState(State{
			Status: StatusDisconnected,
			Phase:  fortivpn.PhaseDisconnected,
		})
	}

	conn, snap, err := s.launcher.Attach(ctx, ticket)
	if err != nil {
		if !ipc.EndpointUnavailable(err) {
			return s.setState(State{
				Status:      StatusDisconnected,
				Phase:       fortivpn.PhaseDisconnected,
				ProfileID:   ticket.ProfileID,
				ProfileName: ticket.ProfileName,
				Error: &Error{
					Message: "无法确认已有隧道状态",
					Hint:    "恢复票据已保留，请重试断开或连接。",
				},
			})
		}
		if err := s.recoverNetwork(ctx, ticket); err != nil {
			return s.setState(State{
				Status:      StatusDisconnected,
				Phase:       fortivpn.PhaseDisconnected,
				ProfileID:   ticket.ProfileID,
				ProfileName: ticket.ProfileName,
				Error: &Error{
					Message: "恢复网络配置失败",
					Hint:    err.Error(),
				},
			})
		}
		return s.State()
	}

	if snap != nil && snap.Phase == fortivpn.PhaseDisconnected {
		if err := s.finishSession(ctx, conn, snap, ticket); err != nil {
			return s.setState(State{
				Status:      StatusDisconnected,
				Phase:       fortivpn.PhaseDisconnected,
				ProfileID:   ticket.ProfileID,
				ProfileName: ticket.ProfileName,
				Error: &Error{
					Message: "恢复网络配置失败",
					Hint:    err.Error(),
				},
			})
		}
		return s.State()
	}

	return s.adoptSession(conn, snap, ticket)
}

// adoptSession publishes the snapshot before consuming subsequent IPC events.
func (s *Service) adoptSession(conn Conn, snap *fortivpn.Event, ticket helperclient.Ticket) State {
	phase := fortivpn.PhaseConnected
	var info *fortivpn.TunnelInfo
	var stats *fortivpn.Stats
	var prompt *fortivpn.Prompt
	if snap != nil {
		if snap.Phase != "" {
			phase = snap.Phase
		}
		info = snap.Info
		stats = snap.Stats
		prompt = snap.Prompt
	}

	recovered := s.setState(State{
		Status:      statusForPhase(phase),
		Phase:       phase,
		ProfileID:   ticket.ProfileID,
		ProfileName: ticket.ProfileName,
		ConnectedAt: ticket.StartedAt,
		Info:        info,
		Stats:       stats,
		Prompt:      prompt,
	})

	s.startEventStream(conn, ticket)
	return recovered
}
