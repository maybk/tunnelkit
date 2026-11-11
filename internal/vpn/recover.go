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
				Error:  errSessionRead(),
			})
		}
		if handled, _ := s.recoverLegacyNetwork(ctx, helperclient.Ticket{}); handled {
			return s.State()
		}
		return s.setState(State{
			Status: StatusDisconnected,
			Phase:  fortivpn.PhaseDisconnected,
		})
	}

	conn, snap, recovered, err := s.attachOrRecoverTicket(ctx, ticket)
	if err != nil {
		s.recordAttachOrRecoverFailure(ticket, recovered, err)
		return s.State()
	}
	if recovered {
		return s.State()
	}

	return s.adoptSession(conn, snap, ticket)
}

func (s *Service) recordAttachOrRecoverFailure(ticket helperclient.Ticket, recovered bool, err error) {
	if !recovered {
		s.failDisconnect(ticket.ProfileID, ticket.ProfileName, errSessionUnconfirmed())
		return
	}
	if st := s.State(); st.Status != StatusDisconnected || st.Error == nil {
		s.failDisconnect(ticket.ProfileID, ticket.ProfileName, errNetworkRecoveryFailed(false, err))
	}
}

// attachOrRecoverTicket 尝试接入票据对应的 helper；若 helper 已退出或处于 PhaseDisconnected，
// 则自动完成会话收尾与网络恢复并返回 recovered=true。
func (s *Service) attachOrRecoverTicket(ctx context.Context, ticket helperclient.Ticket) (conn Conn, snap *fortivpn.Event, recovered bool, err error) {
	conn, snap, attachErr := s.launcher.Attach(ctx, ticket)
	if attachErr != nil {
		if !ipc.EndpointUnavailable(attachErr) {
			return nil, nil, false, attachErr
		}
		if ticket.Protocol == 1 {
			return nil, nil, true, s.finishSession(ctx, nil, &fortivpn.Event{Phase: fortivpn.PhaseDisconnected}, ticket)
		}
		return nil, nil, true, s.recoverNetwork(ctx, ticket)
	}
	if snap != nil && snap.Phase == fortivpn.PhaseDisconnected {
		return nil, nil, true, s.finishSession(ctx, conn, snap, ticket)
	}
	return conn, snap, false, nil
}

// adoptSession 先发布已恢复会话的初始状态快照，再启动后续 IPC 事件流消费。
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

	initial := State{
		Status:      statusForPhase(phase),
		Phase:       phase,
		ProfileID:   ticket.ProfileID,
		ProfileName: ticket.ProfileName,
		ConnectedAt: ticket.StartedAt,
		Info:        info,
		Stats:       stats,
		Prompt:      prompt,
	}

	s.startEventStream(conn, ticket, &initial)
	return s.State()
}
