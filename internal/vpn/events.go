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

type readyResult struct {
	err       *Error
	cleanExit bool
}

func (s *Service) startEventStream(conn Conn, ticket helperclient.Ticket, initial *State) (<-chan readyResult, <-chan struct{}) {
	readyCh := make(chan readyResult, 1)
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
	signalReady := func(res readyResult) { readyOnce.Do(func() { readyCh <- res }) }
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
					s.logMu.Lock()
					_, err := logFile.WriteString(msg.LogLine)
					s.logMu.Unlock()
					if err != nil {
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
				signalReady(readyResult{})
			case fortivpn.PhaseDisconnected:
				// 终态事件不等于进程已退出；恢复票据由确认清理完成的收尾流程删除。
				_ = conn.Close()
				cleanExit := msg.Event != nil && (msg.Event.Err == nil || msg.Event.Err.Code != fortivpn.CodeNetcfgFailed)
				if state.Error != nil {
					signalReady(readyResult{err: state.Error, cleanExit: cleanExit})
				} else {
					signalReady(readyResult{
						err:       &Error{Message: "连接未能建立", Hint: "请在「日志」中查看详细输出。"},
						cleanExit: cleanExit,
					})
				}
				return
			}
		}
		s.mu.RLock()
		active := s.conn == conn
		connecting := s.state.Status == StatusConnecting
		s.mu.RUnlock()
		if active {
			s.appendErrorLog(ticket.ProfileID, errControlStreamLost(connecting), nil)
		}
		state, applied := s.updateState(conn, func(state *State) bool {
			s.conn = nil
			connecting := state.Status == StatusConnecting
			*state = State{
				Status:      StatusDisconnected,
				Phase:       fortivpn.PhaseDisconnected,
				ProfileID:   ticket.ProfileID,
				ProfileName: ticket.ProfileName,
				Error:       errControlStreamLost(connecting),
			}
			return true
		})
		if applied {
			signalReady(readyResult{err: state.Error})
		} else {
			signalReady(readyResult{err: &Error{Message: "连接已取消"}})
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
	if state.Status != StatusConnected {
		state.ConnectedAt, state.Info, state.Stats = "", nil, nil
	}
	if ev.Info != nil && state.Status == StatusConnected {
		cp := *ev.Info
		state.Info = &cp
	}
	if ev.Stats != nil && state.Status == StatusConnected {
		cp := *ev.Stats
		state.Stats = &cp
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
		state.Error = MapError(ev.Err)
	default:
		state.Error = nil
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
