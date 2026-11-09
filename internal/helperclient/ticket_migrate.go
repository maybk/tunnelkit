package helperclient

import (
	"encoding/json"
	"errors"
	"strings"

	"tunnelkit/internal/ipc"
	"tunnelkit/internal/storage"
)

const ticketSchemaVersion = 1

// decodeTicket isolates known unversioned formats from the current encrypted format.
// The caller persists migration atomically before exposing the session token.
func (s *TicketStore) decodeTicket(data []byte) (Ticket, bool, error) {
	payload, legacy, err := storage.DecodeDocument(data, ticketSchemaVersion)
	if err != nil {
		return Ticket{}, false, err
	}
	var wire struct {
		Ticket
		PlainToken string `json:"token"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		return Ticket{}, false, err
	}
	t := wire.Ticket
	if legacy && t.SealedToken.Nonce == "" && t.SealedToken.Ciphertext == "" && wire.PlainToken != "" {
		t.Token, t.Protocol = wire.PlainToken, 1
	} else {
		if !legacy && wire.PlainToken != "" {
			return Ticket{}, false, errors.New("invalid encrypted ticket format")
		}
		t.Token, err = s.sealer.Decrypt(t.SealedToken)
		if err != nil {
			return Ticket{}, false, err
		}
		if legacy && t.Protocol == 0 {
			t.Protocol = 2
		}
	}
	if t.Protocol < 1 || t.Protocol > ipc.ProtocolVersion {
		return Ticket{}, false, ErrProtocolMismatch
	}
	if strings.TrimSpace(t.Endpoint) == "" || strings.TrimSpace(t.Token) == "" {
		return Ticket{}, false, errors.New("invalid session ticket")
	}
	return t, legacy, nil
}
