// Package helperclient 负责三端按需提权启动或重新接管 tunnelkit-helper，
// 并提供类型安全的本地 IPC 客户端以执行 Start / Answer / Stop 与实时事件流订阅。
package helperclient

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"

	"tunnelkit/internal/credentials"
	"tunnelkit/internal/storage"
)

// Ticket 保存跨 GUI 重启重新接入运行中 helper 所需的会话凭据（~/.tunnelkit/run/session.json）。
type Ticket struct {
	Endpoint    string             `json:"endpoint"`
	Token       string             `json:"-"`
	SealedToken credentials.Sealed `json:"sealedToken"`
	PID         int                `json:"pid"`
	ProfileID   string             `json:"profileId"`
	ProfileName string             `json:"profileName"`
	StartedAt   string             `json:"startedAt"`
}

// TicketStore 负责加密持久化与清理 helper IPC 会话凭据。
type TicketStore struct {
	path   string
	sealer *credentials.Sealer
}

// NewTicketStore 创建会话凭据仓库。
func NewTicketStore(rootDir string) *TicketStore {
	return &TicketStore{
		path:   storage.TicketPath(rootDir),
		sealer: credentials.New(rootDir),
	}
}

// Save 加密令牌并原子保存当前会话凭据。
func (s *TicketStore) Save(t Ticket) error {
	if strings.TrimSpace(t.Endpoint) == "" || strings.TrimSpace(t.Token) == "" {
		return errors.New("invalid session ticket")
	}
	sealed, err := s.sealer.Encrypt(t.Token)
	if err != nil {
		return err
	}
	t.SealedToken = sealed
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteFile(s.path, append(data, '\n'), 0o600)
}

// Load 读取并解密已保存的会话凭据。
func (s *TicketStore) Load() (Ticket, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Ticket{}, err
	}
	var t Ticket
	if err := json.Unmarshal(data, &t); err != nil {
		return Ticket{}, err
	}
	t.Token, err = s.sealer.Decrypt(t.SealedToken)
	if err != nil {
		return Ticket{}, err
	}
	if strings.TrimSpace(t.Endpoint) == "" || strings.TrimSpace(t.Token) == "" {
		return Ticket{}, errors.New("invalid session ticket")
	}
	t.SealedToken = credentials.Sealed{}
	return t, nil
}

// Clear 删除已保存的会话凭据。
func (s *TicketStore) Clear() error {
	err := os.Remove(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
