package helperclient

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"tunnelkit/internal/storage"
)

// ReadLegacyJournal reads with desktop permissions; the helper receives data, never a user path.
func ReadLegacyJournal(root string) (json.RawMessage, error) {
	f, err := openCertificateFile(storage.LegacyJournalPath(root))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("旧版网络恢复记录必须是普通文件")
	}
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 64<<10 {
		return nil, errors.New("旧版网络恢复记录过大")
	}
	if len(data) == 0 {
		return json.RawMessage("[]"), nil
	}
	if !json.Valid(data) {
		return nil, errors.New("旧版网络恢复记录格式无效")
	}
	return data, nil
}

// ClearLegacyJournal is called only after confirmed recovery and helper exit.
func ClearLegacyJournal(root string) error {
	err := os.Remove(storage.LegacyJournalPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
