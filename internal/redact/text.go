// Package redact 遮蔽日志文本和结构化属性中的凭据，不改变网络诊断字段。
package redact

import (
	"regexp"
	"strings"
)

const marker = "[REDACTED]"

var (
	sensitiveKey = regexp.MustCompile(`(?i)password|passwd|passphrase|credential|secret|token|cookie|otp|pinentry|authorization|api[_-]?key`)
	assignment   = regexp.MustCompile(`(?i)\b([a-z0-9_.-]*(?:password|passwd|passphrase|credential|secret|token|cookie|otp|pinentry|authorization|api[_-]?key)[a-z0-9_.-]*)["']?\s*[=:]\s*`)
	nextField    = regexp.MustCompile(`^["']?[a-zA-Z0-9_.-]+["']?[ \t]*[=:]`)
)

// Key 判断结构化属性名是否包含凭据字段。
func Key(name string) bool { return sensitiveKey.MatchString(name) }

// Text 遮蔽带引号/空白的键值、JSON 属性及完整认证/Cookie 头。
// 无法确定值边界时遮蔽本行剩余内容，避免只删除凭据的第一个词。
func Text(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out strings.Builder
	for {
		loc := assignment.FindStringSubmatchIndex(text)
		if loc == nil {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:loc[1]])
		key := strings.ToLower(text[loc[2]:loc[3]])
		text = text[loc[1]:]
		end := valueEnd(text)
		quoted := len(text) > 0 && (text[0] == '"' || text[0] == '\'')
		if !quoted && (strings.Contains(key, "authorization") || key == "cookie" || key == "set-cookie") {
			end = len(text)
			if nl := strings.IndexAny(text, "\r\n"); nl >= 0 {
				end = nl
			}
		}
		if quoted {
			out.WriteByte(text[0])
		}
		out.WriteString(marker)
		if quoted {
			out.WriteByte(text[0])
		}
		text = text[end:]
	}
}

func valueEnd(value string) int {
	if strings.HasPrefix(value, marker) {
		return len(marker)
	}
	if value == "" {
		return 0
	}
	if value[0] == '[' || value[0] == '{' {
		return structuredEnd(value)
	}
	if quote := value[0]; quote == '"' || quote == '\'' {
		for i := 1; i < len(value); i++ {
			if value[i] == '\\' {
				i++
				continue
			}
			if value[i] == quote {
				return i + 1
			}
		}
		return len(value)
	}
	for i := 0; i < len(value); i++ {
		if strings.ContainsRune("\r\n&;,)}]<>\"'", rune(value[i])) {
			return i
		}
		if (value[i] == ' ' || value[i] == '\t') && nextField.MatchString(strings.TrimLeft(value[i:], " \t")) {
			return i
		}
	}
	return len(value)
}

// structuredEnd handles nested JSON values without interpreting their contents.
func structuredEnd(value string) int {
	var stack []byte
	var quote byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '[', '{':
			stack = append(stack, c)
		case ']', '}':
			if len(stack) == 0 {
				return len(value)
			}
			opening := stack[len(stack)-1]
			if (c == ']' && opening != '[') || (c == '}' && opening != '{') {
				return len(value)
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1
			}
		}
	}
	return len(value)
}
