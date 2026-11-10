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
	"strings"
	"sync"
	"unicode/utf8"

	"tunnelkit/internal/ipc"
	"tunnelkit/internal/redact"
)

// RedactLogLine masks sensitive tokens, cookies, and credentials before log persistence.
func RedactLogLine(s string) string {
	return redact.Text(s)
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
	return slog.New(&lineLogHandler{mu: &sync.Mutex{}, w: w})
}

type lineLogHandler struct {
	mu     *sync.Mutex
	w      io.Writer
	attrs  []slog.Attr
	groups []string
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
		a = redactLogAttr(a)
		attrParts = append(attrParts, fmt.Sprintf("%s=%v", a.Key, a.Value.Any()))
	}
	for _, a := range h.attrs {
		appendAttr(a)
	}
	var recordAttrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		recordAttrs = append(recordAttrs, a)
		return true
	})
	for _, a := range h.groupAttrs(recordAttrs) {
		appendAttr(a)
	}

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

func redactLogAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if redact.Key(a.Key) {
		a.Value = slog.StringValue("[REDACTED]")
	} else if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		attrs := make([]slog.Attr, len(group))
		for i, child := range group {
			attrs[i] = redactLogAttr(child)
		}
		a.Value = slog.GroupValue(attrs...)
	}
	return a
}

func (h *lineLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cp := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	cp = append(cp, h.attrs...)
	cp = append(cp, h.groupAttrs(attrs)...)
	return &lineLogHandler{mu: h.mu, w: h.w, attrs: cp, groups: h.groups}
}

func (h *lineLogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := append(append([]string(nil), h.groups...), name)
	return &lineLogHandler{mu: h.mu, w: h.w, attrs: h.attrs, groups: groups}
}

func (h *lineLogHandler) groupAttrs(attrs []slog.Attr) []slog.Attr {
	if len(attrs) == 0 {
		return nil
	}
	for i := len(h.groups) - 1; i >= 0; i-- {
		attrs = []slog.Attr{{Key: h.groups[i], Value: slog.GroupValue(attrs...)}}
	}
	return attrs
}

// sessionLogWriter sends only redacted output from NewLogger. Files are written
// by the ordinary desktop process; the privileged helper never accepts log paths.
type sessionLogWriter struct{ state *sessionState }

func (w sessionLogWriter) Write(p []byte) (int, error) {
	originalSize := len(p)
	line := strings.ToValidUTF8(string(p), "")
	if len(line) > 16<<10 {
		size := 16 << 10
		for !utf8.ValidString(line[:size]) {
			size--
		}
		line = line[:size]
	}
	w.state.mu.Lock()
	defer w.state.mu.Unlock()
	if w.state.enqueueFrameLocked(outboundFrame{
		msgType: ipc.TypeLog,
		payload: ipc.LogData{Line: line},
		logLine: line,
	}) {
		return originalSize, nil
	}
	w.state.appendLogBacklogLocked(line)
	return originalSize, nil
}

func (s *sessionState) appendLogBacklogLocked(line string) {
	// Keep a bounded tail across temporary GUI disconnections without privileged
	// filesystem writes. Attach flushes it as a small number of bounded frames.
	s.logBacklog = append(s.logBacklog, line)
	s.logBacklogBytes += len(line)
	for s.logBacklogBytes > 128<<10 {
		s.logBacklogBytes -= len(s.logBacklog[0])
		s.logBacklog[0] = ""
		s.logBacklog = s.logBacklog[1:]
	}
}

func flushLogBacklog(c *ipc.Conn, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	backlog := strings.Join(lines, "")
	for len(backlog) > 0 {
		size := min(16<<10, len(backlog))
		for size > 0 && !utf8.ValidString(backlog[:size]) {
			size--
		}
		if size == 0 {
			break
		}
		if err := c.Send(ipc.TypeLog, 0, ipc.LogData{Line: backlog[:size]}); err != nil {
			return err
		}
		backlog = backlog[size:]
	}
	return nil
}
