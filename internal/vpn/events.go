package vpn

import (
	"sync"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
)

func (s *Service) startEventStream(conn Conn, ticket helperclient.Ticket) (<-chan *Error, <-chan struct{}) {
	readyCh := make(chan *Error, 1)
	doneCh := make(chan struct{})

	s.mu.Lock()
	oldConn := s.conn
	s.conn = conn
	s.streamDone = doneCh
	s.mu.Unlock()

	if oldConn != nil && oldConn != conn {
		_ = oldConn.Close()
	}

	var readyOnce sync.Once
	signalReady := func(vpnErr *Error) {
		readyOnce.Do(func() {
			readyCh <- vpnErr
		})
	}

	go func() {
		defer close(doneCh)

		var sawDisconnected bool
		for msg := range conn.Events() {
			s.mu.RLock()
			isCurrent := s.conn == conn
			cur := s.state
			s.mu.RUnlock()
			if !isCurrent {
				continue
			}

			if msg.Prompt != nil {
				if cur.Status == StatusDisconnecting {
					continue
				}
				cur.Status = StatusConnecting
				cur.Phase = fortivpn.PhaseWaitingInput
				cp := *msg.Prompt
				cur.Prompt = &cp
				s.setState(cur)
				continue
			}

			if msg.Event == nil {
				continue
			}
			ev := msg.Event
			if cur.Status == StatusDisconnecting && ev.Phase != fortivpn.PhaseDisconnected {
				continue
			}
			cur.ProfileID = ticket.ProfileID
			cur.ProfileName = ticket.ProfileName
			cur.Phase = ev.Phase
			cur.Status = statusForPhase(ev.Phase)
			if ev.Phase != fortivpn.PhaseWaitingInput {
				cur.Prompt = nil
			}
			if ev.Info != nil {
				cp := *ev.Info
				cur.Info = &cp
			}
			if ev.Stats != nil {
				cp := *ev.Stats
				cur.Stats = &cp
			}

			switch ev.Phase {
			case fortivpn.PhaseConnected:
				if cur.ConnectedAt == "" {
					if ticket.StartedAt != "" {
						cur.ConnectedAt = ticket.StartedAt
					} else {
						cur.ConnectedAt = time.Now().Format(time.RFC3339)
					}
				}
				cur.Error = nil
				s.setState(cur)
				signalReady(nil)

			case fortivpn.PhaseDisconnected:
				sawDisconnected = true
				_ = s.tickets.Clear()
				s.mu.Lock()
				if s.conn == conn {
					s.conn = nil
				}
				s.mu.Unlock()
				_ = conn.Close()

				var vpnErr *Error
				if ev.Err != nil {
					vpnErr = MapError(ev.Err)
				}
				cur.ConnectedAt = ""
				cur.Info = nil
				cur.Stats = nil
				cur.Prompt = nil
				cur.Error = vpnErr
				s.setState(cur)
				if vpnErr == nil {
					signalReady(&Error{Message: "连接未能建立", Hint: "请在「日志」中查看详细输出。"})
				} else {
					signalReady(vpnErr)
				}
				return

			default:
				if ev.Err != nil {
					cur.Error = MapError(ev.Err)
				}
				s.setState(cur)
			}
		}

		// Events channel closed without an explicit PhaseDisconnected event.
		s.mu.Lock()
		isCurrent := s.conn == conn
		if isCurrent {
			s.conn = nil
		}
		cur := s.state
		s.mu.Unlock()

		if !sawDisconnected && isCurrent && cur.Status != StatusDisconnected && cur.Status != StatusDisconnecting {
			_ = s.tickets.Clear()
			dropErr := &Error{
				Message: "连接已意外断开",
				Hint:    "隧道后台服务已退出，请在「日志」中查看详细输出。",
			}
			if cur.Status == StatusConnecting {
				dropErr = &Error{
					Message: "连接未能建立",
					Hint:    "隧道后台服务已退出，请在「日志」中查看详细输出。",
				}
			}
			s.setState(State{
				Status:      StatusDisconnected,
				Phase:       fortivpn.PhaseDisconnected,
				ProfileID:   ticket.ProfileID,
				ProfileName: ticket.ProfileName,
				Error:       dropErr,
			})
			signalReady(dropErr)
		} else {
			signalReady(&Error{Message: "连接已取消"})
		}
	}()

	return readyCh, doneCh
}

func (s *Service) waitStreamDone(doneCh <-chan struct{}, timeout time.Duration) {
	if doneCh == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-doneCh:
	case <-timer.C:
	}
}
