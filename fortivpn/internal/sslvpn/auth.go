package sslvpn

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"tunnelkit/fortivpn"
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

// Login authenticates against the FortiGate gateway (password + optional 2FA, or SAML)
// and returns the SVPNCOOKIE session value.
func (c *Client) Login(ctx context.Context, promptFn func(ctx context.Context, p fortivpn.Prompt) (string, error)) (string, error) {
	// Each authentication attempt starts without cookies from an earlier login.
	c.hc.Jar, _ = cookiejar.New(nil)
	if c.cfg.Auth == fortivpn.AuthSAML {
		return c.AuthenticateSAML(ctx, promptFn)
	}

	form := BuildLoginForm(c.cfg.Username, c.cfg.Password.Reveal(), c.cfg.Realm)
	res, respCookie, err := c.postLogin(ctx, form)
	if err != nil {
		return "", err
	}

	if !(res.Ret == 1 && respCookie != "") && res.IsChallenge() {
		chal := res.Challenge()
		if promptFn == nil {
			return "", fortivpn.NewError(fortivpn.CodeOTPRequired, chal.Message)
		}
		kind := fortivpn.PromptOTP
		msg := chal.Message
		if chal.FTMPush() {
			kind = fortivpn.PromptFTMPush
			if msg == "" {
				msg = "Approve FortiToken Mobile push notification or enter token code"
			}
		} else if msg == "" {
			msg = "Enter one-time verification code"
		}

		promptID := "otp-" + rand.Text()
		code, err := promptFn(ctx, fortivpn.Prompt{
			ID:      promptID,
			Kind:    kind,
			Message: msg,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", context.Canceled
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return "", fortivpn.WrapError(fortivpn.CodePromptTimeout, "timed out waiting for verification code", err)
			}
			return "", fortivpn.WrapError(fortivpn.CodeOTPRequired, "verification code prompt cancelled", err)
		}

		res, respCookie, err = c.postLogin(ctx, BuildChallengeForm(c.cfg.Username, code, c.cfg.Realm, chal))
		if err != nil {
			var fe *fortivpn.Error
			if errors.As(err, &fe) && fe.Code == fortivpn.CodeAuthFailed {
				return "", fortivpn.NewError(fortivpn.CodeOTPInvalid, "gateway rejected the verification code")
			}
			return "", err
		}
		if res.Ret != 1 || respCookie == "" {
			return "", fortivpn.NewError(fortivpn.CodeOTPInvalid, "gateway rejected the verification code")
		}
	}

	if res.Ret != 1 {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, fmt.Sprintf("gateway login failed (ret=%d)", res.Ret))
	}

	cookie := respCookie
	if cookie == "" {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, "gateway did not issue SVPNCOOKIE")
	}
	return cookie, nil
}

// Logout sends GET /remote/logout with a 3-second timeout.
func (c *Client) Logout(ctx context.Context, cookie string) error {
	if cookie == "" {
		return nil
	}
	logoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(logoutCtx, http.MethodGet, c.baseURL()+PathLogout, nil)
	if err != nil {
		return err
	}
	c.setCommonHeaders(req)
	req.Header.Set("Cookie", CookieName+"="+cookie)

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (c *Client) postLogin(ctx context.Context, form string) (LoginResult, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+PathLoginCheck, strings.NewReader(form))
	if err != nil {
		return LoginResult{}, "", fortivpn.WrapError(fortivpn.CodeGatewayUnreachable, "failed to create login request", err)
	}
	c.setCommonHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.hc.Do(req)
	if err != nil {
		return LoginResult{}, "", c.classifyTransportError(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return LoginResult{}, "", ctxErr
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusMethodNotAllowed {
		return LoginResult{}, "", fortivpn.NewError(fortivpn.CodeAuthFailed, fmt.Sprintf("gateway rejected login with HTTP %d", resp.StatusCode))
	}
	if readErr != nil {
		return LoginResult{}, "", c.classifyTransportError(readErr)
	}

	bodyStr := string(body)
	if strings.Contains(bodyStr, "sslvpn_login_permission_denied") || strings.Contains(bodyStr, "Permission denied") {
		return LoginResult{}, "", fortivpn.NewError(fortivpn.CodeAuthFailed, "gateway reported permission denied")
	}

	cookie := responseCookie(resp)
	res, err := ParseLoginResult(bodyStr)
	if err != nil {
		// FortiOS 6.x and custom portal configurations may respond to /remote/logincheck
		// with an HTML/JS redirect while issuing SVPNCOOKIE in Set-Cookie.
		if cookie != "" && strings.HasPrefix(strings.TrimSpace(bodyStr), "<") {
			return LoginResult{
				Ret:    1,
				Redir:  "/sslvpn/portal.html",
				Fields: map[string]string{"ret": "1"},
			}, cookie, nil
		}
		return LoginResult{}, "", fortivpn.WrapError(fortivpn.CodeAuthFailed, "unrecognized login response from gateway", err)
	}
	return res, cookie, nil
}

// responseCookie inspects this request's response chain, never cached cookies
// from a previous authentication exchange.
func responseCookie(resp *http.Response) string {
	for resp != nil {
		cookies := resp.Cookies()
		for i := len(cookies) - 1; i >= 0; i-- {
			ck := cookies[i]
			if ck.Name != CookieName {
				continue
			}
			if ck.Value == "" || ck.MaxAge < 0 || (!ck.Expires.IsZero() && !ck.Expires.After(time.Now())) {
				return ""
			}
			return ck.Value
		}
		if resp.Request == nil {
			break
		}
		resp = resp.Request.Response
	}
	return ""
}
