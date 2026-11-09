package vpn

import (
	"context"
	"errors"
	"os"

	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/storage"
)

// recoverLegacyNetwork 独立检查旧版恢复记录，不将票据缺失视为网络已清理。
func (s *Service) recoverLegacyNetwork(ctx context.Context, prior helperclient.Ticket) (bool, error) {
	_, err := os.Lstat(storage.LegacyJournalPath(s.rootDir))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	return true, s.recoverNetwork(ctx, prior)
}
