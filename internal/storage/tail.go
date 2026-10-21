package storage

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const DefaultTailBytes int64 = 64 * 1024

var (
	cookieValuePattern = regexp.MustCompile(`(?i)(SVPNCOOKIE=|Cookie:\s*)([^\s;]+)`)
	sensitiveKeys      = []string{"password", "passphrase", "secret", "token", "cookie", "otp", "pinentry"}
)

// LogTail 是日志文件尾部的脱敏内容。
type LogTail struct {
	Content string
	Exists  bool
	ModTime time.Time
}

// TailFile 读取日志文件最后 maxBytes 字节并脱敏；文件不存在时返回 Exists=false。
func TailFile(path string, maxBytes int64) (LogTail, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultTailBytes
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return LogTail{}, nil
	}
	if err != nil {
		return LogTail{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return LogTail{}, err
	}
	var start int64
	if info.Size() > maxBytes {
		start = info.Size() - maxBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return LogTail{}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return LogTail{}, err
	}
	content := strings.TrimRight(Redact(string(data)), "\n")
	if start > 0 {
		content = "... 已省略更早的日志\n" + content
	}
	return LogTail{Content: content, Exists: true, ModTime: info.ModTime()}, nil
}

// ResetFile 创建或清空指定日志文件。
func ResetFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// Redact 遮蔽文本中的密码、令牌与会话 Cookie。
func Redact(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if redacted, ok := redactAssignment(line); ok {
			lines[i] = redacted
			continue
		}
		lines[i] = cookieValuePattern.ReplaceAllString(line, "${1}******")
	}
	return strings.Join(lines, "\n")
}

func redactAssignment(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	prefix, _, ok := strings.Cut(line, "=")
	if !ok {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(prefix))
	for _, token := range sensitiveKeys {
		if strings.Contains(key, token) {
			return strings.TrimRight(prefix, " \t") + " = ******", true
		}
	}
	return "", false
}
