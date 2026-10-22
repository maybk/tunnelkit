package vpn

import (
	"context"

	"tunnelkit/fortivpn"
)

// Recover 在应用启动时通过 run/session.json 重新接入正在运行的 helper，或静默清理失效会话。
func (s *Service) Recover(ctx context.Context) State {
	release, err := s.begin()
	if err != nil {
		return s.State()
	}
	defer release()

	s.tickets.CleanLegacyArtifacts()

	ticket, err := s.tickets.Load()
	if err != nil {
		_ = s.tickets.Clear()
		return s.setState(State{
			Status: StatusDisconnected,
			Phase:  fortivpn.PhaseDisconnected,
		})
	}

	conn, snap, err := s.launcher.Attach(ctx, ticket)
	if err != nil {
		_ = s.tickets.Clear()
		return s.setState(State{
			Status: StatusDisconnected,
			Phase:  fortivpn.PhaseDisconnected,
		})
	}

	if snap != nil && snap.Phase == fortivpn.PhaseDisconnected {
		_ = conn.Close()
		_ = s.tickets.Clear()
		return s.setState(State{
			Status: StatusDisconnected,
			Phase:  fortivpn.PhaseDisconnected,
		})
	}

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
