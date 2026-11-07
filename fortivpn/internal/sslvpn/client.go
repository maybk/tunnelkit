package sslvpn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
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
// It clears any cached gateway IP first so reconnects after a network change re-resolve DNS.
func (c *Client) Probe(ctx context.Context) error {
	c.mu.Lock()
	c.gatewayIP = netip.Addr{}
	c.mu.Unlock()

	conn, err := c.dialTLS(ctx, "tcp", c.hostPort())
	if err != nil {
		return c.classifyTransportError(err)
	}
	_ = conn.Close()
	return nil
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

func (c *Client) dialTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	dialAddr := addr
	if addr == c.hostPort() {
		if gwIP := c.GatewayIP(); gwIP.IsValid() {
			dialAddr = fmt.Sprintf("%s:%d", gwIP.String(), c.cfg.Port)
		}
	}
	var (
		rawConn net.Conn
		err     error
	)
	if dialAddr != addr {
		// Bound the pinned IP dial attempt so DNS fallback still has time budget if the IP became unreachable.
		pinnedCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		rawConn, err = c.dialer.DialContext(pinnedCtx, network, dialAddr)
		cancel()
		if err != nil {
			c.mu.Lock()
			c.gatewayIP = netip.Addr{}
			c.mu.Unlock()
			rawConn, err = c.dialer.DialContext(ctx, network, addr)
		}
	} else {
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
