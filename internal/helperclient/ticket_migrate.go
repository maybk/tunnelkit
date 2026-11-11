package helperclient

import (
	"bytes"
	"errors"
	"strings"

	"tunnelkit/internal/ipc"
	"tunnelkit/internal/storage"
)

const ticketSchemaVersion = 1

type legacyTicketWire struct {
	Ticket
	PlainToken string `json:"token"`
}

// decodeTicket isolates known unversioned formats from the current encrypted format.
// The caller persists migration atomically before exposing the session token.
func (s *TicketStore) decodeTicket(data []byte) (Ticket, bool, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Ticket{}, false, errors.New("empty session ticket")
	}
	payload, legacy, err := storage.DecodeDocument(data, ticketSchemaVersion)
	if err != nil {
		return Ticket{}, false, err
	}
	if !legacy {
		t, err := ipc.DecodeStrict[Ticket](payload)
		if err != nil {
			return Ticket{}, false, err
		}
		if t.Protocol < 1 || t.Protocol > ipc.ProtocolVersion {
			return Ticket{}, false, ErrProtocolMismatch
		}
		if strings.TrimSpace(t.Endpoint) == "" || t.SealedToken.Nonce == "" || t.SealedToken.Ciphertext == "" {
			return Ticket{}, false, errors.New("invalid session ticket")
		}
		t.Token, err = s.sealer.Decrypt(t.SealedToken)
		if err != nil {
			return Ticket{}, false, err
		}
		if strings.TrimSpace(t.Token) == "" {
			return Ticket{}, false, errors.New("invalid session ticket")
		}
		return t, false, nil
	}

	wire, err := ipc.DecodeStrict[legacyTicketWire](payload)
	if err != nil {
		return Ticket{}, false, err
	}
	t := wire.Ticket
	if t.SealedToken.Nonce == "" && t.SealedToken.Ciphertext == "" && wire.PlainToken != "" {
		if t.Protocol == 0 {
			t.Protocol = 1
		} else if t.Protocol != 1 {
			return Ticket{}, false, errors.New("invalid legacy session ticket")
		}
		t.Token = wire.PlainToken
	} else {
		if wire.PlainToken != "" {
			return Ticket{}, false, errors.New("invalid encrypted ticket format")
		}
		if t.Protocol == 0 {
			t.Protocol = 2
		}
		if t.Protocol < 1 || t.Protocol > ipc.ProtocolVersion {
			return Ticket{}, false, ErrProtocolMismatch
		}
		if strings.TrimSpace(t.Endpoint) == "" || t.SealedToken.Nonce == "" || t.SealedToken.Ciphertext == "" {
			return Ticket{}, false, errors.New("invalid session ticket")
		}
		t.Token, err = s.sealer.Decrypt(t.SealedToken)
		if err != nil {
			return Ticket{}, false, err
		}
	}
	if t.Protocol < 1 || t.Protocol > ipc.ProtocolVersion {
		return Ticket{}, false, ErrProtocolMismatch
	}
	if strings.TrimSpace(t.Endpoint) == "" || strings.TrimSpace(t.Token) == "" {
		return Ticket{}, false, errors.New("invalid session ticket")
	}
	return t, true, nil
}
