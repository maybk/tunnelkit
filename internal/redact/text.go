// Package redact 遮蔽日志文本和结构化属性中的凭据与本机路径用户名，不改变网络诊断字段。
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
	uriCreds     = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^\s/@"'<>]+:[^\s/@"'<>]*)@`)
	authScheme   = regexp.MustCompile(`(?i)\b(Bearer|Basic)[ \t]+([^\s"',;\]\}\\\(\)<>]+)`)
)

// Key 判断结构化属性名是否包含凭据字段。
func Key(name string) bool { return sensitiveKey.MatchString(name) }

// Text 遮蔽带引号/空白的键值、JSON 属性、完整认证/Cookie 头、URI 内嵌凭据、
// 独立 Bearer/Basic 令牌及三端用户主目录路径中的操作系统用户名。
// 无法确定值边界时遮蔽本行剩余内容，避免只删除凭据的第一个词。
func Text(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out strings.Builder
	for {
		loc := assignment.FindStringSubmatchIndex(text)
		if loc == nil {
			out.WriteString(text)
			break
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
	res := redactURICredentials(out.String())
	res = redactAuthSchemeTokens(res)
	return UserPaths(res)
}

func redactURICredentials(text string) string {
	if !strings.Contains(text, "://") || !strings.Contains(text, "@") {
		return text
	}
	return uriCreds.ReplaceAllString(text, "${1}"+marker+"@")
}

func redactAuthSchemeTokens(text string) string {
	matches := authScheme.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	cursor := 0
	for _, m := range matches {
		scheme := text[m[2]:m[3]]
		token := text[m[4]:m[5]]
		if strings.HasPrefix(text[m[4]:], marker) {
			continue
		}
		if strings.EqualFold(scheme, "basic") && !looksLikeBasicCredential(token) {
			continue
		}
		if out.Cap() == 0 {
			out.Grow(len(text))
		}
		out.WriteString(text[cursor:m[4]])
		out.WriteString(marker)
		cursor = m[5]
	}
	if cursor == 0 {
		return text
	}
	out.WriteString(text[cursor:])
	return out.String()
}

// looksLikeBasicCredential 区分 Base64 编码的 HTTP Basic 凭据与普通英文短语（如 "Basic auth failed"）。
func looksLikeBasicCredential(token string) bool {
	if len(token) < 8 {
		return false
	}
	hasUpper, hasLower, hasDigit, hasB64Sym := false, false, false, false
	for i := 0; i < len(token); i++ {
		c := token[i]
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '+' || c == '/' || c == '=' || c == '_' || c == '-':
			hasB64Sym = true
		default:
			return false
		}
	}
	return hasB64Sym || hasDigit || (hasUpper && hasLower)
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
