package sslvpn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
)

// Client manages the HTTPS control plane and TLS tunnel connection to a FortiGate gateway.
type Client struct {
	cfg            fortivpn.Config
	dialer         *net.Dialer
	rootCAs        *x509.CertPool
	tlsCfg         *tls.Config
	hc             *http.Client
	SAMLListenAddr string
	configXMLHook  func(raw []byte)

	mu        sync.Mutex
	gatewayIP netip.Addr
	promptSeq atomic.Uint64
}

// ClientOption configures optional parameters for Client (such as custom RootCAs in tests).
type ClientOption func(*Client)

// WithRootCAs sets a custom Root CA pool for TLS verification (used in tests).
func WithRootCAs(pool *x509.CertPool) ClientOption {
	return func(c *Client) { c.rootCAs = pool }
}

// WithSAMLListenAddr overrides the localhost callback address for SAML tests.
func WithSAMLListenAddr(addr string) ClientOption {
	return func(c *Client) { c.SAMLListenAddr = addr }
}

// WithConfigXMLHook registers a callback receiving the raw fortisslvpn_xml payload.
func WithConfigXMLHook(fn func(raw []byte)) ClientOption {
	return func(c *Client) { c.configXMLHook = fn }
}

// NewClient constructs an sslvpn Client from a validated fortivpn.Config.
func NewClient(cfg fortivpn.Config, dialer *net.Dialer, opts ...ClientOption) (*Client, error) {
	cfg = cfg.Normalized()
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	}
	c := &Client{
		cfg:    cfg,
		dialer: dialer,
	}
	for _, opt := range opts {
		opt(c)
	}

	tlsCfg, err := BuildTLSConfig(cfg.Host, cfg.TrustedCerts, cfg.ClientCert, cfg.LegacyTLS, c.rootCAs)
	if err != nil {
		return nil, err
	}
	c.tlsCfg = tlsCfg

	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return c.dialTLS(ctx, network, addr)
		},
		DisableKeepAlives: false,
	}

	c.hc = &http.Client{
		Transport: tr,
		Jar:       jar,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			// Never follow redirects across hosts.
			if !strings.EqualFold(req.URL.Hostname(), cfg.Host) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	return c, nil
}

// GatewayIP returns the resolved remote IPv4 address of the gateway from the most recent connection.
func (c *Client) GatewayIP() netip.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gatewayIP
}

// Probe performs a TLS handshake to the gateway to verify reachability and certificate trust.
func (c *Client) Probe(ctx context.Context) error {
	conn, err := c.dialTLS(ctx, "tcp", c.hostPort())
	if err != nil {
		return c.classifyTransportError(err)
	}
	_ = conn.Close()
	return nil
}

// Login authenticates against the FortiGate gateway (via password + optional 2FA, or SAML)
// and returns the SVPNCOOKIE session value.
func (c *Client) Login(ctx context.Context, promptFn func(ctx context.Context, p fortivpn.Prompt) (string, error)) (string, error) {
	if c.cfg.Auth == fortivpn.AuthSAML {
		return c.AuthenticateSAML(ctx, promptFn)
	}

	form := BuildLoginForm(c.cfg.Username, c.cfg.Password.Reveal(), c.cfg.Realm)
	res, respCookie, err := c.postLogin(ctx, form)
	if err != nil {
		return "", err
	}

	if res.IsChallenge() {
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

		promptID := fmt.Sprintf("otp-%d", c.promptSeq.Add(1))
		code, err := promptFn(ctx, fortivpn.Prompt{
			ID:      promptID,
			Kind:    kind,
			Message: msg,
		})
		if err != nil {
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
		if res.IsChallenge() || res.Ret != 1 {
			return "", fortivpn.NewError(fortivpn.CodeOTPInvalid, "gateway rejected the verification code")
		}
	}

	if res.Ret != 1 {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, fmt.Sprintf("gateway login failed (ret=%d)", res.Ret))
	}

	cookie := respCookie
	if cookie == "" {
		cookie = c.jarCookie()
	}
	if cookie == "" {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, "gateway did not issue SVPNCOOKIE")
	}
	return cookie, nil
}

// FetchTunnelConfig retrieves and parses /remote/fortisslvpn_xml (with fallback to /remote/fortisslvpn).
func (c *Client) FetchTunnelConfig(ctx context.Context, cookie string) (TunnelConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+PathConfigXML, nil)
	if err != nil {
		return TunnelConfig{}, fortivpn.WrapError(fortivpn.CodeGatewayUnreachable, "failed to create tunnel config request", err)
	}
	c.setCommonHeaders(req)
	req.Header.Set("Cookie", CookieName+"="+cookie)

	resp, err := c.hc.Do(req)
	if err != nil {
		return TunnelConfig{}, c.classifyTransportError(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return TunnelConfig{}, ctxErr
	}
	if readErr != nil {
		return TunnelConfig{}, c.classifyTransportError(readErr)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return TunnelConfig{}, fortivpn.NewError(fortivpn.CodeTunnelDenied, fmt.Sprintf("gateway denied tunnel configuration with HTTP %d", resp.StatusCode))
	}
	if resp.StatusCode == http.StatusNotFound {
		return c.fetchLegacyConfig(ctx, cookie)
	}
	if resp.StatusCode != http.StatusOK {
		return TunnelConfig{}, fortivpn.NewError(fortivpn.CodeTunnelDenied, fmt.Sprintf("unexpected HTTP %d from fortisslvpn_xml", resp.StatusCode))
	}

	if c.configXMLHook != nil {
		c.configXMLHook(body)
	}
	cfg, err := ParseConfigXML(body)
	if err != nil {
		if legacyCfg, legacyErr := c.fetchLegacyConfig(ctx, cookie); legacyErr == nil && len(legacyCfg.SplitRoutes) > 0 {
			return legacyCfg, nil
		}
		return TunnelConfig{}, fortivpn.WrapError(fortivpn.CodeTunnelDenied, "failed to parse tunnel XML configuration", err)
	}
	return cfg, nil
}

func (c *Client) fetchLegacyConfig(ctx context.Context, cookie string) (TunnelConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+PathConfigLegacy, nil)
	if err != nil {
		return TunnelConfig{}, err
	}
	c.setCommonHeaders(req)
	req.Header.Set("Cookie", CookieName+"="+cookie)

	resp, err := c.hc.Do(req)
	if err != nil {
		return TunnelConfig{}, c.classifyTransportError(err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		return TunnelConfig{}, c.classifyTransportError(readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return TunnelConfig{}, fortivpn.NewError(fortivpn.CodeTunnelDenied, "SSL-VPN tunnel mode is not available on this gateway")
	}
	return ParseLegacyConfigHTML(body)
}

// OpenTunnel opens a fresh TLS connection to the gateway and sends GET /remote/sslvpn-tunnel.
func (c *Client) OpenTunnel(ctx context.Context, cookie string) (*TunnelConn, error) {
	conn, err := c.dialTLS(ctx, "tcp", c.hostPort())
	if err != nil {
		return nil, c.classifyTransportError(err)
	}

	reqBytes := TunnelRequest(c.cfg.Host, c.cfg.Port, cookie, c.userAgent())
	if err := conn.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, c.classifyTransportError(err)
	}
	if _, err := conn.Write(reqBytes); err != nil {
		_ = conn.Close()
		return nil, c.classifyTransportError(err)
	}
	_ = conn.SetWriteDeadline(time.Time{})

	return NewTunnelConn(conn), nil
}

// Logout sends GET /remote/logout on a fresh HTTPS request with a 3-second timeout.
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

	cookie := c.extractCookie(resp)
	res, err := ParseLoginResult(bodyStr)
	if err != nil {
		// FortiOS 6.x and custom portal configurations may respond to /remote/logincheck
		// with an HTML/JS redirect (e.g. document.location='/sslvpn/portal.html') while
		// issuing SVPNCOOKIE in Set-Cookie instead of returning "ret=1,redir=...".
		if cookie != "" {
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

func (c *Client) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	dialAddr := addr
	if addr == c.hostPort() {
		if gwIP := c.GatewayIP(); gwIP.IsValid() {
			dialAddr = fmt.Sprintf("%s:%d", gwIP.String(), c.cfg.Port)
		}
	}
	rawConn, err := c.dialer.DialContext(ctx, network, dialAddr)
	if err != nil && dialAddr != addr {
		rawConn, err = c.dialer.DialContext(ctx, network, addr)
	}
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := rawConn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	if remoteTCP, ok := rawConn.RemoteAddr().(*net.TCPAddr); ok {
		if ip, ok := netip.AddrFromSlice(remoteTCP.IP); ok {
			c.mu.Lock()
			c.gatewayIP = ip.Unmap()
			c.mu.Unlock()
		}
	}

	tlsConn := tls.Client(rawConn, c.tlsCfg.Clone())
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return tlsConn, nil
}

func (c *Client) classifyTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var fe *fortivpn.Error
	if errors.As(err, &fe) {
		return fe
	}
	return fortivpn.WrapError(fortivpn.CodeGatewayUnreachable, err.Error(), err)
}

func (c *Client) extractCookie(resp *http.Response) string {
	var last string
	for _, ck := range resp.Cookies() {
		if ck.Name == CookieName && ck.Value != "" {
			last = ck.Value
		}
	}
	if last != "" {
		return last
	}
	return c.jarCookie()
}

func (c *Client) jarCookie() string {
	if c.hc.Jar == nil {
		return ""
	}
	u, err := url.Parse(c.baseURL())
	if err != nil {
		return ""
	}
	for _, ck := range c.hc.Jar.Cookies(u) {
		if ck.Name == CookieName && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

func (c *Client) setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-store, no-cache, must-revalidate")
}

func (c *Client) userAgent() string {
	if c.cfg.UserAgent != "" {
		return c.cfg.UserAgent
	}
	return fortivpn.DefaultUserAgent
}

func (c *Client) hostPort() string {
	return fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("https://%s:%d", c.cfg.Host, c.cfg.Port)
}
