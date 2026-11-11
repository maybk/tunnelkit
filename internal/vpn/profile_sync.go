package vpn

import (
	"errors"
	"os"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/profile"
)

// InUse 判断指定配置是否处于活动连接、连接握手中，或仍持有待清理的会话恢复记录。
func (s *Service) InUse(profileID string) bool {
	if profileID == "" {
		return false
	}
	s.mu.RLock()
	active := s.state.Active(profileID) || (s.inFlightProfileID == profileID && s.cancelConnect != nil)
	curProfileID := s.state.ProfileID
	s.mu.RUnlock()
	if active {
		return true
	}
	ticket, err := s.tickets.Load()
	if err == nil {
		return ticket.ProfileID == profileID
	}
	if !errors.Is(err, os.ErrNotExist) {
		return curProfileID == profileID
	}
	return s.hasLegacyJournal() && curProfileID == profileID
}

// SyncSavedProfile 在配置保存或信任证书后同步名称，并在无待恢复会话时清除已过期的连接错误。
func (s *Service) SyncSavedProfile(p profile.Profile) {
	if p.ID == "" {
		return
	}
	pendingRecovery := s.hasPendingRecovery()
	s.updateState(nil, func(state *State) bool {
		if state.ProfileID != p.ID {
			return false
		}
		changed := false
		if p.Name != "" && state.ProfileName != p.Name {
			state.ProfileName = p.Name
			changed = true
		}
		if state.Status == StatusDisconnected && state.Error != nil && !pendingRecovery {
			state.Error = nil
			changed = true
		}
		return changed
	})
}

// ForgetProfile 在配置已删除且无待恢复会话时清理其残留的断开状态与错误记录。
func (s *Service) ForgetProfile(profileID string) {
	if profileID == "" || s.hasPendingRecovery() {
		return
	}
	s.updateState(nil, func(state *State) bool {
		if state.Status != StatusDisconnected || state.ProfileID != profileID {
			return false
		}
		*state = State{
			Status: StatusDisconnected,
			Phase:  fortivpn.PhaseDisconnected,
		}
		return true
	})
}

func (s *Service) hasPendingRecovery() bool {
	return s.tickets.HasPending() || s.hasLegacyJournal()
}
