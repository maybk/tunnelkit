package vpn

import (
	"context"
	"errors"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
)

// recoverNetwork retries a stale or failed session's protected journal without
// opening a VPN connection or reading the profile's credentials.
func (s *Service) recoverNetwork(ctx context.Context, prior helperclient.Ticket) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.waitForHelper(ctx); err != nil {
		return err
	}
	saved := false
	conn, ticket, err := s.launcher.Launch(ctx, helperclient.LaunchRequest{
		ProfileID:    prior.ProfileID,
		ProfileName:  prior.ProfileName,
		RecoveryOnly: true,
		OnReady: func(ticket helperclient.Ticket) error {
			if err := s.tickets.Save(ticket); err != nil {
				return err
			}
			saved = true
			return nil
		},
	})
	if err != nil {
		return err
	}
	if !saved {
		if err := s.tickets.Save(ticket); err != nil {
			_ = conn.Close()
			return err
		}
	}
	s.setState(State{
		Status:      StatusDisconnecting,
		Phase:       fortivpn.PhaseDisconnecting,
		ProfileID:   prior.ProfileID,
		ProfileName: prior.ProfileName,
	})
	_, done := s.startEventStream(conn, ticket)
	if err := s.waitStreamDoneContext(ctx, done); err != nil {
		return err
	}
	if err := s.waitForHelper(ctx); err != nil {
		return err
	}
	if final := s.State(); final.Error != nil {
		return errors.New(final.Error.Message)
	}
	return s.tickets.Clear()
}

func (s *Service) finishSession(ctx context.Context, conn Conn, snapshot *fortivpn.Event, ticket helperclient.Ticket) error {
	if process, ok := conn.(interface{ WaitForExit(context.Context) error }); ok {
		s.mu.Lock()
		s.waitHelper = process.WaitForExit
		s.mu.Unlock()
	}
	_ = conn.Close()
	if err := s.waitForHelper(ctx); err != nil {
		return err
	}
	if snapshot.Err != nil && snapshot.Err.Code == fortivpn.CodeNetcfgFailed {
		return s.recoverNetwork(ctx, ticket)
	}
	if err := s.tickets.Clear(); err != nil {
		return err
	}
	s.setState(State{
		Status:      StatusDisconnected,
		Phase:       fortivpn.PhaseDisconnected,
		ProfileID:   ticket.ProfileID,
		ProfileName: ticket.ProfileName,
	})
	return nil
}
