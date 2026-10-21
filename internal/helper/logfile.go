// Package helper implements the single-session privileged daemon (`tunnelkit-helper`)
// that runs `fortivpn/engine` and communicates with the unprivileged GUI over `internal/ipc`.
package helper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var sensitiveKeyValueRegex = regexp.MustCompile(`(?i)\b(SVPNCOOKIE|credential|password|passwd|token|otp)=([^\s&;,"']+)`)

// RedactLogLine masks sensitive tokens, cookies, and credentials before log persistence.
func RedactLogLine(s string) string {
	return sensitiveKeyValueRegex.ReplaceAllString(s, `$1=[REDACTED]`)
}

// OpenLogFile opens (or creates) the session log file in append mode with 0600 permissions.
func OpenLogFile(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// NewLogger creates a human-readable line logger ("2006-01-02 15:04:05 LEVEL Message")
// with automatic sensitive-token scrubbing, suitable for tailing in the TunnelKit desktop UI.
func NewLogger(w io.Writer) *slog.Logger {
	if w == nil {
		w = io.Discard
	}
	return slog.New(&lineLogHandler{w: w})
}

type lineLogHandler struct {
	mu    sync.Mutex
	w     io.Writer
	attrs []slog.Attr
}

func (h *lineLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *lineLogHandler) Handle(_ context.Context, r slog.Record) error {
	ts := r.Time.Format("2006-01-02 15:04:05")
	lvl := fmt.Sprintf("%-5s", r.Level.String())

	var attrParts []string
	appendAttr := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			return
		}
		keyLower := strings.ToLower(a.Key)
		if keyLower == "password" || keyLower == "credential" || keyLower == "cookie" || keyLower == "token" || keyLower == "otp" {
			attrParts = append(attrParts, a.Key+"=[REDACTED]")
			return
		}
		attrParts = append(attrParts, fmt.Sprintf("%s=%v", a.Key, a.Value.Any()))
	}
	for _, a := range h.attrs {
		appendAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(a)
		return true
	})

	msg := r.Message
	if len(attrParts) > 0 {
		msg = fmt.Sprintf("%s (%s)", msg, strings.Join(attrParts, " "))
	}
	line := RedactLogLine(fmt.Sprintf("%s %s %s\n", ts, lvl, msg))

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, line)
	return err
}

func (h *lineLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	cp = append(cp, h.attrs...)
	cp = append(cp, attrs...)
	return &lineLogHandler{w: h.w, attrs: cp}
}

func (h *lineLogHandler) WithGroup(_ string) slog.Handler {
	return h
}
