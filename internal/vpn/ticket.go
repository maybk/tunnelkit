package vpn

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/storage"
)

// TicketStore 负责持久化与清理 helper IPC 会话凭据（run/session.json）。
type TicketStore struct {
	rootDir string
	path    string
}

func NewTicketStore(rootDir string) *TicketStore {
	return &TicketStore{
		rootDir: rootDir,
		path:    storage.TicketPath(rootDir),
	}
}

func (s *TicketStore) Save(t helperclient.Ticket) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(s.path, append(data, '\n'), 0o600)
}

func (s *TicketStore) Load() (helperclient.Ticket, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return helperclient.Ticket{}, err
	}
	var t helperclient.Ticket
	if err := json.Unmarshal(data, &t); err != nil {
		return helperclient.Ticket{}, err
	}
	if strings.TrimSpace(t.Endpoint) == "" || strings.TrimSpace(t.Token) == "" {
		return helperclient.Ticket{}, errors.New("invalid session ticket")
	}
	return t, nil
}

func (s *TicketStore) Clear() error {
	err := os.Remove(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// CleanLegacyArtifacts 清理从旧版本升级时可能遗留在 run/ 目录下的 *.conf 临时文件。
func (s *TicketStore) CleanLegacyArtifacts() {
	runDir := storage.RunDir(s.rootDir)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".conf") {
			_ = os.Remove(filepath.Join(runDir, entry.Name()))
		}
	}
}
