package vpn

import (
	"context"
	"errors"
	"os"

	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/storage"
)

func (s *Service) hasLegacyJournal() bool {
	_, err := os.Lstat(storage.LegacyJournalPath(s.rootDir))
	return !errors.Is(err, os.ErrNotExist)
}

// recoverLegacyNetwork 独立检查旧版恢复记录，不将票据缺失视为网络已清理；
// 若恢复失败且状态尚未承载明确错误，则统一收敛至断开异常态。
func (s *Service) recoverLegacyNetwork(ctx context.Context, prior helperclient.Ticket) (bool, error) {
	_, err := os.Lstat(storage.LegacyJournalPath(s.rootDir))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err == nil {
		err = s.recoverNetwork(ctx, prior)
	}
	if err != nil {
		if st := s.State(); st.Status != StatusDisconnected || st.Error == nil {
			s.failDisconnect(prior.ProfileID, prior.ProfileName, errNetworkRecoveryFailed(true, err))
		}
	}
	return true, err
}
