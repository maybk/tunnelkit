// Package ipc implements the length-prefixed JSON framing and authenticated
// local transport (Unix domain socket with LOCAL_PEERCRED on macOS, Named Pipe
// with DACL on Windows) between the TunnelKit GUI and tunnelkit-helper.
package ipc

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"tunnelkit/fortivpn"
)

const (
	// ProtocolVersion is incremented whenever the helper <-> GUI wire schema changes.
	ProtocolVersion = 1

	// MaxFrameSize bounds a single IPC JSON message to 1 MiB.
	MaxFrameSize = 1 << 20
)

// MessageType identifies the kind of IPC message inside an Envelope.
type MessageType string

const (
	TypeHello   MessageType = "hello"
	TypeWelcome MessageType = "welcome"
	TypeStart   MessageType = "start"
	TypeAnswer  MessageType = "answer"
	TypeStop    MessageType = "stop"
	TypeEvent   MessageType = "event"
	TypePrompt  MessageType = "prompt"
	TypeResult  MessageType = "result"
)

// Envelope is the outer wire frame for all IPC messages.
type Envelope struct {
	Type MessageType     `json:"type"`
	ID   uint64          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// HelloData is sent by the GUI as the first message on a newly connected IPC stream.
type HelloData struct {
	Protocol int    `json:"protocol"`
	Token    string `json:"token"`
}

// WelcomeData is returned by the helper after verifying HelloData.
type WelcomeData struct {
	Protocol int             `json:"protocol"`
	Version  string          `json:"version"`
	PID      int             `json:"pid"`
	Snapshot *fortivpn.Event `json:"snapshot,omitempty"`
}

// StartData instructs the helper to begin the VPN session with Config.
type StartData struct {
	Config fortivpn.Config `json:"config"`
}

// AnswerData delivers the user's response (or cancellation) for an active Prompt.
type AnswerData struct {
	PromptID string `json:"promptId"`
	Value    string `json:"value,omitempty"`
	Cancel   bool   `json:"cancel,omitempty"`
}

// StopData requests graceful VPN disconnection.
type StopData struct{}

// ResultData acknowledges a request (start, answer, stop) with an optional error.
type ResultData struct {
	ID    uint64 `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

// NewEnvelope marshals payload into an Envelope.
func NewEnvelope(msgType MessageType, id uint64, payload any) (Envelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Envelope{}, fmt.Errorf("ipc: marshal %s payload: %w", msgType, err)
		}
		raw = b
	}
	return Envelope{
		Type: msgType,
		ID:   id,
		Data: raw,
	}, nil
}

// DecodeStrict unmarshals raw JSON into T while rejecting unknown fields.
func DecodeStrict[T any](raw json.RawMessage) (T, error) {
	var out T
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

// GenerateToken returns a cryptographically random 32-byte hex session token.
func GenerateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// HashToken computes the lowercase 64-character SHA-256 hex digest of token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// VerifyTokenHash checks whether token hashes to expectedHexHash in constant time.
func VerifyTokenHash(token, expectedHexHash string) bool {
	expected := strings.ToLower(strings.TrimSpace(expectedHexHash))
	if len(expected) != 64 {
		return false
	}
	actual := HashToken(token)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
