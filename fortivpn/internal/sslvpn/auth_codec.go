package sslvpn

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// formField preserves field order for FortiOS challenge responses.
type formField struct{ name, value string }

func encodeAuthForm(fields []formField) string {
	var body strings.Builder
	for index, field := range fields {
		if index != 0 {
			body.WriteByte('&')
		}
		body.WriteString(url.QueryEscape(field.name))
		body.WriteByte('=')
		body.WriteString(url.QueryEscape(field.value))
	}
	return body.String()
}

// BuildLoginForm encodes the password login fields required by FortiOS.
func BuildLoginForm(username, password, realm string) string {
	return encodeAuthForm([]formField{
		{"username", username}, {"credential", password}, {"realm", realm},
		{"ajax", "1"}, {"just_logged_in", "1"},
	})
}

// LoginResult holds the status, redirect and gateway-provided response fields.
type LoginResult struct {
	Ret    int
	Redir  string
	Fields map[string]string
}

// ParseLoginResult accepts comma-delimited fields. Inside chal_msg, only known
// protocol fields delimit the prompt; unknown key-like text remains visible.
func ParseLoginResult(line string) (LoginResult, error) {
	fields := make(map[string]string)
	var name string
	var value strings.Builder
	save := func() {
		if name != "" {
			fields[name] = strings.TrimSpace(value.String())
		}
		value.Reset()
	}
	for part := range strings.SplitSeq(strings.TrimSpace(line), ",") {
		key, content, pair := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		if pair && key != "" && !strings.ContainsAny(key, " \t\r\n<>") && (name != "chal_msg" || isLoginResponseField(key)) {
			save()
			name = key
			value.WriteString(content)
		} else if name != "" {
			value.WriteByte(',')
			value.WriteString(part)
		}
	}
	save()
	status, err := strconv.Atoi(fields["ret"])
	if err != nil || status < 0 {
		return LoginResult{}, fmt.Errorf("sslvpn: login response has no valid ret field")
	}
	return LoginResult{Ret: status, Redir: fields["redir"], Fields: fields}, nil
}

func isLoginResponseField(name string) bool {
	switch name {
	case "ret", "redir", "tokeninfo", "chal_msg", "reqid", "polid", "grp", "portal", "peer", "magic":
		return true
	default:
		return false
	}
}

// Challenge contains the user prompt and the opaque fields needed for its reply.
type Challenge struct {
	Message   string
	TokenInfo string
	Echo      map[string]string
}

// FTMPush identifies a FortiToken Mobile challenge that supports push approval.
func (c Challenge) FTMPush() bool { return c.TokenInfo == "ftm_push" }

// IsChallenge reports potential second-factor metadata; Login prioritizes a
// successful result carrying a fresh session cookie.
func (r LoginResult) IsChallenge() bool {
	_, present := r.Fields["tokeninfo"]
	return r.Ret != 0 && present
}

// Challenge selects the protocol echo fields and sanitizes the displayed prompt.
func (r LoginResult) Challenge() Challenge {
	echo := make(map[string]string)
	for name, value := range r.Fields {
		switch name {
		case "reqid", "polid", "grp", "portal", "peer", "magic":
			echo[name] = value
		}
	}
	return Challenge{Message: SanitizePromptMessage(r.Fields["chal_msg"]), TokenInfo: r.Fields["tokeninfo"], Echo: echo}
}

// BuildChallengeForm keeps opaque fields ordered and puts magic last for code
// verification, or ftmpush last for push approval. Extra echo keys are ignored.
func BuildChallengeForm(username, code, realm string, challenge Challenge) string {
	fields := []formField{{"username", username}, {"code", code}, {"realm", realm}}
	for _, name := range [...]string{"reqid", "polid", "grp", "portal", "peer"} {
		if value, present := challenge.Echo[name]; present {
			fields = append(fields, formField{name, value})
		}
	}
	switch {
	case challenge.FTMPush() && code == "":
		fields = append(fields, formField{"ftmpush", "1"})
	default:
		if magic, present := challenge.Echo["magic"]; present {
			fields = append(fields, formField{"magic", magic})
		}
	}
	return encodeAuthForm(fields)
}

// SanitizePromptMessage URL-unescapes, replaces control characters with spaces, and caps length at 256 runes.
func SanitizePromptMessage(msg string) string {
	if msg == "" {
		return ""
	}
	if unescaped, err := url.QueryUnescape(msg); err == nil {
		msg = unescaped
	}
	var b strings.Builder
	count := 0
	for _, r := range msg {
		if count >= 256 {
			break
		}
		if unicode.IsControl(r) {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
		count++
	}
	return strings.TrimSpace(b.String())
}
