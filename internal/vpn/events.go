package vpn

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/storage"
)

func (s *Service) startEventStream(conn Conn, ticket helperclient.Ticket, initial *State) (<-chan *Error, <-chan struct{}) {
	readyCh := make(chan *Error, 1)
	doneCh := make(chan struct{})
	var oldConn Conn
	s.updateState(nil, func(state *State) bool {
		oldConn = s.conn
		s.conn = conn
		if process, ok := conn.(interface{ WaitForExit(context.Context) error }); ok {
			s.waitHelper = process.WaitForExit
		}
		s.streamDone = doneCh
		if initial != nil {
			*state = *initial
		}
		return true
	})
	if oldConn != nil && oldConn != conn {
		_ = oldConn.Close()
	}

	var readyOnce sync.Once
	signalReady := func(vpnErr *Error) { readyOnce.Do(func() { readyCh <- vpnErr }) }
	go func() {
		defer close(doneCh)
		logFile, logErr := os.OpenFile(storage.LogPath(s.rootDir, ticket.ProfileID), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if logErr != nil {
			slog.Warn("无法打开连接日志", "error", logErr)
		} else {
			defer logFile.Close()
		}
		for msg := range conn.Events() {
			if msg.LogLine != "" {
				s.mu.RLock()
				current := s.conn == conn
				s.mu.RUnlock()
				if current && logFile != nil {
					if _, err := logFile.WriteString(msg.LogLine); err != nil {
						slog.Warn("写入连接日志失败", "error", err)
						_ = logFile.Close()
						logFile = nil
					}
				}
				continue
			}
			state, applied := s.updateState(conn, func(state *State) bool {
				if !applyMessage(state, msg, ticket) {
					return false
				}
				if state.Phase == fortivpn.PhaseDisconnected {
					s.conn = nil
				}
				return true
			})
			if !applied {
				continue
			}
			switch state.Phase {
			case fortivpn.PhaseConnected:
				signalReady(nil)
			case fortivpn.PhaseDisconnected:
				// 终态事件不等于进程已退出；恢复票据由确认清理完成的收尾流程删除。
				_ = conn.Close()
				if state.Error != nil {
					signalReady(state.Error)
				} else {
					signalReady(&Error{Message: "连接未能建立", Hint: "请在「日志」中查看详细输出。"})
				}
				return
			}
		}
		state, applied := s.updateState(conn, func(state *State) bool {
			s.conn = nil
			message := "连接已意外断开"
			if state.Status == StatusConnecting {
				message = "连接未能建立"
			}
			*state = State{
				Status: StatusDisconnected, Phase: fortivpn.PhaseDisconnected,
				ProfileID: ticket.ProfileID, ProfileName: ticket.ProfileName,
				Error: &Error{Message: message, Hint: "隧道控制通道已断开，恢复票据已保留；请重试断开或连接，并查看日志。"},
			}
			return true
		})
		if applied {
			signalReady(state.Error)
		} else {
			signalReady(&Error{Message: "连接已取消"})
		}
	}()
	return readyCh, doneCh
}

// applyMessage 只计算当前连接的状态；调用方在同一临界区校验连接归属并发布。
func applyMessage(state *State, msg helperclient.Message, ticket helperclient.Ticket) bool {
	if msg.Prompt != nil {
		if state.Status != StatusConnecting || !promptPhase(state.Phase) || !msg.Prompt.Valid() {
			return false
		}
		if state.Prompt != nil && state.Prompt.ID == msg.Prompt.ID {
			return false
		}
		cp := *msg.Prompt
		state.Phase, state.Prompt = fortivpn.PhaseWaitingInput, &cp
		return true
	}
	ev := msg.Event
	if ev == nil {
		return false
	}
	if state.Status == StatusDisconnecting && ev.Phase != fortivpn.PhaseDisconnected {
		return false
	}
	// 已连接只能进入重连或收尾；统计消息不能把重认证中的会话重新标为已连接。
	if state.Status == StatusConnected && ev.Phase != fortivpn.PhaseConnected && ev.Phase != fortivpn.PhaseReconnecting && ev.Phase != fortivpn.PhaseDisconnecting && ev.Phase != fortivpn.PhaseDisconnected {
		return false
	}
	if ev.Phase == fortivpn.PhaseConnected && ev.Stats != nil && ev.Info == nil && state.Status != StatusConnected {
		return false
	}
	state.ProfileID, state.ProfileName = ticket.ProfileID, ticket.ProfileName
	state.Phase, state.Status = ev.Phase, statusForPhase(ev.Phase)
	state.Prompt = nil
	if ev.Phase == fortivpn.PhaseWaitingInput && ev.Prompt != nil && ev.Prompt.Valid() {
		cp := *ev.Prompt
		state.Prompt = &cp
	}
	if ev.Info != nil {
		cp := *ev.Info
		state.Info = &cp
	}
	if ev.Stats != nil {
		cp := *ev.Stats
		state.Stats = &cp
	}
	if ev.Err != nil {
		state.Error = MapError(ev.Err)
	}
	switch ev.Phase {
	case fortivpn.PhaseConnected:
		if state.ConnectedAt == "" {
			state.ConnectedAt = ticket.StartedAt
			if state.ConnectedAt == "" {
				state.ConnectedAt = time.Now().Format(time.RFC3339)
			}
		}
		state.Error = nil
	case fortivpn.PhaseDisconnected:
		state.ConnectedAt, state.Info, state.Stats = "", nil, nil
		state.Error = MapError(ev.Err)
	}
	return true
}

func promptPhase(phase fortivpn.Phase) bool {
	switch phase {
	case fortivpn.PhaseConnecting, fortivpn.PhaseAuthenticating, fortivpn.PhaseWaitingInput, fortivpn.PhaseReconnecting:
		return true
	default:
		return false
	}
}
