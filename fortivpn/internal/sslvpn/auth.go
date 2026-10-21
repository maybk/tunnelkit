package sslvpn

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// Well-known Fortinet SSL-VPN HTTP endpoints and session cookie name.
const (
	PathLoginCheck   = "/remote/logincheck"
	PathConfigXML    = "/remote/fortisslvpn_xml"
	PathConfigLegacy = "/remote/fortisslvpn"
	PathTunnel       = "/remote/sslvpn-tunnel"
	PathLogout       = "/remote/logout"
	PathSAMLStart    = "/remote/saml/start?redirect=1"
	PathSAMLAuthID   = "/remote/saml/auth_id"
	CookieName       = "SVPNCOOKIE"
)

// BuildLoginForm encodes the application/x-www-form-urlencoded body POSTed to /remote/logincheck.
func BuildLoginForm(username, password, realm string) string {
	v := url.Values{}
	v.Set("username", username)
	v.Set("credential", password)
	v.Set("realm", realm)
	v.Set("ajax", "1")
	v.Set("just_logged_in", "1")
	return v.Encode()
}

// LoginRequest is a decoded /remote/logincheck form (used by fortitest.Gateway).
type LoginRequest struct {
	Username string
	Password string
	Realm    string
}

// ParseLoginForm decodes a POSTed login form body.
func ParseLoginForm(body string) (LoginRequest, error) {
	v, err := url.ParseQuery(body)
	if err != nil {
		return LoginRequest{}, fmt.Errorf("sslvpn: malformed login form: %w", err)
	}
	return LoginRequest{
		Username: v.Get("username"),
		Password: v.Get("credential"),
		Realm:    v.Get("realm"),
	}, nil
}

// BuildLoginSuccess formats the response line for a successful login.
func BuildLoginSuccess(redir string) string {
	if redir == "" {
		redir = PathConfigXML
	}
	return "ret=1,redir=" + redir
}

// LoginResult represents a decoded /remote/logincheck response.
type LoginResult struct {
	Ret    int
	Redir  string
	Fields map[string]string
}

// ParseLoginResult decodes the comma-separated key=value response from /remote/logincheck.
func ParseLoginResult(line string) (LoginResult, error) {
	res := LoginResult{Ret: -1, Fields: map[string]string{}}
	for _, part := range strings.Split(strings.TrimSpace(line), ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		res.Fields[k] = v
		switch k {
		case "ret":
			if n, err := strconv.Atoi(v); err == nil {
				res.Ret = n
			}
		case "redir":
			res.Redir = v
		}
	}
	if res.Ret < 0 {
		return LoginResult{}, fmt.Errorf("sslvpn: login response has no ret field")
	}
	return res, nil
}

// challengeEcho lists the fields echoed from a ret=2 challenge back to the gateway.
// Order is critical: "magic" must be last because FTM push drops it and appends ftmpush=1.
var challengeEcho = []string{"reqid", "polid", "grp", "portal", "peer", "magic"}

// Challenge represents a second-factor authentication prompt from the gateway.
type Challenge struct {
	Message   string
	TokenInfo string
	Echo      map[string]string
}

// FTMPush reports whether the gateway offered FortiToken Mobile push approval.
func (c Challenge) FTMPush() bool { return c.TokenInfo == "ftm_push" }

// IsChallenge reports whether the login response requests a second factor.
func (r LoginResult) IsChallenge() bool {
	_, ok := r.Fields["tokeninfo"]
	return ok
}

// Challenge extracts the second-factor challenge details from a LoginResult.
func (r LoginResult) Challenge() Challenge {
	c := Challenge{
		Message:   SanitizePromptMessage(r.Fields["chal_msg"]),
		TokenInfo: r.Fields["tokeninfo"],
		Echo:      map[string]string{},
	}
	for _, k := range challengeEcho {
		if v, ok := r.Fields[k]; ok {
			c.Echo[k] = v
		}
	}
	return c
}

// BuildChallengeResponse formats a ret=2 challenge response line (for fortitest.Gateway).
func BuildChallengeResponse(tokenInfo, message string, echo map[string]string) string {
	var b strings.Builder
	b.WriteString("ret=2,tokeninfo=")
	b.WriteString(tokenInfo)
	for _, k := range challengeEcho {
		if v, ok := echo[k]; ok {
			fmt.Fprintf(&b, ",%s=%s", k, v)
		}
	}
	if message != "" {
		fmt.Fprintf(&b, ",chal_msg=%s", message)
	}
	return b.String()
}

// BuildChallengeForm hand-builds the second-stage POST body so field order is
// preserved and "magic" remains last (or is replaced by &ftmpush=1 for FTM push).
func BuildChallengeForm(username, code, realm string, c Challenge) string {
	var b strings.Builder
	fmt.Fprintf(&b, "username=%s&code=%s&realm=%s",
		url.QueryEscape(username), url.QueryEscape(code), url.QueryEscape(realm))

	push := c.FTMPush() && code == ""
	for _, k := range challengeEcho {
		v, ok := c.Echo[k]
		if !ok {
			continue
		}
		if k == "magic" && push {
			continue
		}
		fmt.Fprintf(&b, "&%s=%s", k, url.QueryEscape(v))
	}
	if push {
		b.WriteString("&ftmpush=1")
	}
	return b.String()
}

// ChallengeRequest is a decoded second-stage form body (used by fortitest.Gateway).
type ChallengeRequest struct {
	Username string
	Code     string
	Realm    string
	Echo     map[string]string
	FTMPush  bool
}

// ParseChallengeForm decodes a POSTed second-stage challenge form.
func ParseChallengeForm(body string) (ChallengeRequest, error) {
	v, err := url.ParseQuery(body)
	if err != nil {
		return ChallengeRequest{}, fmt.Errorf("sslvpn: malformed challenge form: %w", err)
	}
	req := ChallengeRequest{
		Username: v.Get("username"),
		Code:     v.Get("code"),
		Realm:    v.Get("realm"),
		Echo:     map[string]string{},
		FTMPush:  v.Get("ftmpush") == "1",
	}
	for _, k := range challengeEcho {
		if v.Has(k) {
			req.Echo[k] = v.Get(k)
		}
	}
	return req, nil
}

// IsChallengeForm reports whether body is a second-stage challenge submission.
func IsChallengeForm(body string) bool {
	v, err := url.ParseQuery(body)
	if err != nil {
		return false
	}
	return v.Has("reqid")
}

// SanitizePromptMessage strips control characters and caps length to 256 runes.
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
