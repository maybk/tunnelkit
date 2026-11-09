// Package helperclient 负责三端按需提权启动或重新接管 tunnelkit-helper，
// 并提供类型安全的本地 IPC 客户端以执行 Start / Answer / Stop 与实时事件流订阅。
package helperclient

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"tunnelkit/internal/credentials"
	"tunnelkit/internal/ipc"
	"tunnelkit/internal/storage"
)

// Ticket 保存跨 GUI 重启重新接入运行中 helper 所需的会话凭据（~/.tunnelkit/run/session.json）。
type Ticket struct {
	Protocol    int                `json:"protocol,omitempty"`
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
	if t.Protocol == 0 {
		t.Protocol = ipc.ProtocolVersion
	}
	if t.Protocol < 1 || t.Protocol > ipc.ProtocolVersion {
		return ErrProtocolMismatch
	}
	sealed, err := s.sealer.Encrypt(t.Token)
	if err != nil {
		return err
	}
	t.SealedToken = sealed
	data, err := storage.MarshalDocument(ticketSchemaVersion, t)
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
	t, migrate, err := s.decodeTicket(data)
	if err != nil {
		return Ticket{}, err
	}
	if migrate {
		if err := s.Save(t); err != nil {
			return Ticket{}, err
		}
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
