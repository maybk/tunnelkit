package redact

import (
	"regexp"
	"strings"
)

var (
	macHomePathPattern = regexp.MustCompile(`(/Users/)([^/\r\n"',;\)\]\}\\]+)`)
	linHomePathPattern = regexp.MustCompile(`(/home/)([^/\r\n"',;\)\]\}\\]+)`)
	winHomePathPattern = regexp.MustCompile(`(?i)([A-Z]:(?:\\+|/)Users(?:\\+|/))([^\\/\r\n"',;\)\]\}]+)`)
	safeHomeDirNames   = map[string]struct{}{
		"shared":       {},
		"guest":        {},
		"public":       {},
		"default":      {},
		"default user": {},
		"all users":    {},
		"[redacted]":   {},
	}
)

// UserPaths 遮蔽 macOS（/Users/<name>/）、Linux（/home/<name>/）与 Windows
// （C:\Users\<name>\、C:/Users/<name>/ 及 JSON 转义 C:\\Users\\<name>\\）路径中的本机用户名。
func UserPaths(text string) string {
	if text == "" {
		return ""
	}
	maskWithBoundary := func(re *regexp.Regexp, in string, requireUnixBoundary bool) string {
		matches := re.FindAllStringSubmatchIndex(in, -1)
		if len(matches) == 0 {
			return in
		}
		var b strings.Builder
		cursor := 0
		for _, m := range matches {
			matchStart, matchEnd := m[0], m[1]
			if matchStart < cursor {
				continue
			}
			if requireUnixBoundary && !hasValidUnixPathLeftBoundary(in, matchStart) {
				continue
			}
			if !requireUnixBoundary && !hasValidWindowsPathLeftBoundary(in, matchStart) {
				continue
			}
			segStart, segEnd := m[4], m[5]
			// 仅当用户名段后紧跟路径分隔符时允许其中含空格，避免吞掉裸路径后的正常文本。
			followedBySep := segEnd < len(in) && (in[segEnd] == '/' || in[segEnd] == '\\')
			if !followedBySep {
				if ws := strings.IndexAny(in[segStart:segEnd], " \t"); ws >= 0 {
					segEnd = segStart + ws
					matchEnd = segEnd
				}
			}
			user := in[segStart:segEnd]
			if user == "" || strings.HasPrefix(in[segStart:], marker) {
				continue
			}
			if _, safe := safeHomeDirNames[strings.ToLower(strings.TrimSpace(user))]; safe {
				continue
			}
			if b.Cap() == 0 {
				b.Grow(len(in))
			}
			b.WriteString(in[cursor:m[3]])
			b.WriteString(marker)
			cursor = matchEnd
		}
		if cursor == 0 {
			return in
		}
		b.WriteString(in[cursor:])
		return b.String()
	}
	out := maskWithBoundary(macHomePathPattern, text, true)
	out = maskWithBoundary(linHomePathPattern, out, true)
	out = maskWithBoundary(winHomePathPattern, out, false)
	return out
}

func hasValidUnixPathLeftBoundary(in string, matchStart int) bool {
	if matchStart == 0 || isPathPrefixBoundary(in[matchStart-1]) {
		return true
	}
	if matchStart >= 2 && in[matchStart-2] == '\\' && (in[matchStart-1] == 'n' || in[matchStart-1] == 'r' || in[matchStart-1] == 't') {
		return true
	}
	if matchStart >= 7 && strings.EqualFold(in[matchStart-7:matchStart], "file://") {
		return true
	}
	const apfsPrefix = "/System/Volumes/Data"
	return matchStart >= len(apfsPrefix) && strings.HasSuffix(in[:matchStart], apfsPrefix)
}

func hasValidWindowsPathLeftBoundary(in string, matchStart int) bool {
	if matchStart == 0 {
		return true
	}
	if matchStart >= 2 && in[matchStart-2] == '\\' && (in[matchStart-1] == 'n' || in[matchStart-1] == 'r' || in[matchStart-1] == 't') {
		return true
	}
	c := in[matchStart-1]
	return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
}

func isPathPrefixBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '"', '\'', '`', '=', ':', '(', '[', '{', ',', ';', '<':
		return true
	default:
		return false
	}
}
