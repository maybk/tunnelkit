// Package fortitest provides an in-process fake FortiGate SSL-VPN gateway and
// an in-memory TUN device for rootless end-to-end testing of fortivpn.
//
// Portions of gateway.go are adapted from xen0bit/veepin (internal/fortinet/server.go,
// MIT License, commit bcac1789f0a7eaa7687b2777d6d6765fb87158e0).
package fortitest

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"tunnelkit/fortivpn/internal/ppp"
	"tunnelkit/fortivpn/internal/sslvpn"
)

// GatewayConfig configures a test FortiGate SSL-VPN server.
type GatewayConfig struct {
	Users                  map[string]string
	OTPCode                map[string]string
	FTMPush                map[string]bool
	SAMLToken              string
	AssignedIP             netip.Addr
	ServerIP               netip.Addr
	DNS                    []netip.Addr
	IPCPDNS                []netip.Addr
	DNSDomains             []string
	SplitRoutes            []netip.Prefix
	DuplicateRoutes        []netip.Prefix
	TunnelDenied           bool
	SendUpgradeHTTP200     bool
	ReconnectWithoutReauth bool
	FortiOS                string
}

type challengeRecord struct {
	username string
	ftmPush  bool
}

type activeTunnel struct {
	conn net.Conn
	srv  *ppp.ServerSession
}

// Gateway is an in-process HTTPS + PPP-over-TLS fake FortiGate server.
type Gateway struct {
	cfg    GatewayConfig
	server *httptest.Server

	mu         sync.Mutex
	challenges map[string]challengeRecord
	sessions   map[string]string // cookie -> username
	tunnels    map[*activeTunnel]struct{}
	dropEcho   bool

	tunnelCount atomic.Int32
	logoutCount atomic.Int32
}

// NewGateway starts a new TLS-enabled fake FortiGate gateway on 127.0.0.1.
func NewGateway(cfg GatewayConfig) *Gateway {
	if !cfg.AssignedIP.IsValid() {
		cfg.AssignedIP = netip.MustParseAddr("10.212.0.10")
	}
	if !cfg.ServerIP.IsValid() {
		cfg.ServerIP = netip.MustParseAddr("169.254.2.1")
	}
	if cfg.FortiOS == "" {
		cfg.FortiOS = "FG100F"
	}
	gw := &Gateway{
		cfg:        cfg,
		challenges: make(map[string]challengeRecord),
		sessions:   make(map[string]string),
		tunnels:    make(map[*activeTunnel]struct{}),
	}
	gw.server = httptest.NewTLSServer(gw)
	return gw
}

// Close shuts down the test gateway and all active tunnel connections.
func (g *Gateway) Close() {
	g.DropActiveTunnels()
	g.server.Close()
}

// Host returns the gateway's listening IP/hostname.
func (g *Gateway) Host() string {
	host, _, _ := net.SplitHostPort(g.server.Listener.Addr().String())
	return host
}

// Port returns the gateway's listening TCP port.
func (g *Gateway) Port() int {
	_, portStr, _ := net.SplitHostPort(g.server.Listener.Addr().String())
	p, _ := strconv.Atoi(portStr)
	return p
}

// CertFingerprint returns the lowercase 64-hex SHA-256 digest of the server's leaf certificate.
func (g *Gateway) CertFingerprint() string {
	cert := g.server.Certificate()
	return sslvpn.CertFingerprint(cert.Raw)
}

// RootCAs returns a *x509.CertPool containing the server's self-signed certificate.
func (g *Gateway) RootCAs() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(g.server.Certificate())
	return pool
}

// TunnelCount returns the total number of upgraded tunnel connections established so far.
func (g *Gateway) TunnelCount() int {
	return int(g.tunnelCount.Load())
}

// LogoutCount returns the number of /remote/logout calls received.
func (g *Gateway) LogoutCount() int {
	return int(g.logoutCount.Load())
}

// ActiveTunnels returns the number of currently open PPP tunnel connections.
func (g *Gateway) ActiveTunnels() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.tunnels)
}

// DropActiveTunnels forcibly closes all currently open PPP tunnel connections
// without invalidating session cookies, simulating a network interruption.
func (g *Gateway) DropActiveTunnels() {
	g.mu.Lock()
	list := make([]*activeTunnel, 0, len(g.tunnels))
	for t := range g.tunnels {
		list = append(list, t)
	}
	g.mu.Unlock()

	for _, t := range list {
		_ = t.conn.Close()
	}
}

// InvalidateCookies clears all issued session cookies so the next reconnect requires re-login.
func (g *Gateway) InvalidateCookies() {
	g.mu.Lock()
	defer g.mu.Unlock()
	clear(g.sessions)
}

// SetDropEcho configures whether active and future tunnels ignore LCP Echo-Requests.
func (g *Gateway) SetDropEcho(drop bool) {
	g.mu.Lock()
	g.dropEcho = drop
	for t := range g.tunnels {
		t.srv.SetDropEcho(drop)
	}
	g.mu.Unlock()
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case sslvpn.PathLoginCheck:
		g.handleLogin(w, r)
	case "/remote/saml/auth_id":
		g.handleSAMLAuthID(w, r)
	case sslvpn.PathConfigXML:
		g.handleConfigXML(w, r)
	case sslvpn.PathTunnel:
		g.handleTunnel(w, r)
	case sslvpn.PathLogout:
		g.handleLogout(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (g *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	body := string(bodyBytes)

	if sslvpn.IsChallengeForm(body) {
		g.handleChallenge(w, body)
		return
	}

	req, err := sslvpn.ParseLoginForm(body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	wantPass, ok := g.cfg.Users[req.Username]
	if !ok || wantPass != req.Password {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ret=0,redir=/remote/login?&err=sslvpn_login_permission_denied")
		return
	}

	_, needsOTP := g.cfg.OTPCode[req.Username]
	supportsPush := g.cfg.FTMPush[req.Username]
	if needsOTP || supportsPush {
		reqID := randomHex(8)
		g.mu.Lock()
		g.challenges[reqID] = challengeRecord{
			username: req.Username,
			ftmPush:  supportsPush,
		}
		g.mu.Unlock()

		tokenInfo := "totp"
		msg := "Enter your OTP code"
		if supportsPush {
			tokenInfo = "ftm_push"
			msg = "Approve FortiToken push or enter code"
		}
		_, _ = io.WriteString(w, sslvpn.BuildChallengeResponse(tokenInfo, msg, map[string]string{
			"reqid":  reqID,
			"polid":  "1",
			"grp":    "sslvpn",
			"portal": "full-access",
			"magic":  "magic-" + reqID,
		}))
		return
	}

	g.issueSession(w, req.Username)
}

func (g *Gateway) handleChallenge(w http.ResponseWriter, body string) {
	req, err := sslvpn.ParseChallengeForm(body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	reqID := req.Echo["reqid"]
	g.mu.Lock()
	rec, ok := g.challenges[reqID]
	delete(g.challenges, reqID)
	g.mu.Unlock()

	if !ok || rec.username != req.Username {
		http.Error(w, "ret=4", http.StatusForbidden)
		return
	}

	if req.FTMPush {
		if !rec.ftmPush {
			http.Error(w, "ret=4", http.StatusForbidden)
			return
		}
		g.issueSession(w, rec.username)
		return
	}

	wantCode := g.cfg.OTPCode[rec.username]
	if wantCode == "" || req.Code != wantCode {
		_, _ = io.WriteString(w, "ret=0")
		return
	}
	g.issueSession(w, rec.username)
}

func (g *Gateway) handleSAMLAuthID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if g.cfg.SAMLToken == "" || id != g.cfg.SAMLToken {
		http.Error(w, "invalid saml id", http.StatusForbidden)
		return
	}
	g.issueSession(w, "saml-user")
}

func (g *Gateway) issueSession(w http.ResponseWriter, username string) {
	cookie := randomHex(16)
	g.mu.Lock()
	g.sessions[cookie] = username
	g.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:  sslvpn.CookieName,
		Value: cookie,
		Path:  "/",
	})
	_, _ = io.WriteString(w, sslvpn.BuildLoginSuccess(sslvpn.PathConfigXML))
}

func (g *Gateway) handleConfigXML(w http.ResponseWriter, r *http.Request) {
	if !g.validCookie(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	routes := append([]netip.Prefix(nil), g.cfg.SplitRoutes...)
	routes = append(routes, g.cfg.DuplicateRoutes...)

	xmlBytes := sslvpn.BuildConfigXML(sslvpn.TunnelConfig{
		AssignedIP:             g.cfg.AssignedIP,
		DNS:                    g.cfg.DNS,
		DNSDomains:             g.cfg.DNSDomains,
		SplitRoutes:            routes,
		FortiOS:                g.cfg.FortiOS,
		ReconnectWithoutReauth: g.cfg.ReconnectWithoutReauth,
	})
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(xmlBytes)
}

func (g *Gateway) handleTunnel(w http.ResponseWriter, r *http.Request) {
	if !g.validCookie(r) {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}
	if g.cfg.TunnelDenied {
		http.Error(w, "tunnel mode disabled", http.StatusForbidden)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}

	g.tunnelCount.Add(1)
	if g.cfg.SendUpgradeHTTP200 {
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n"))
	}

	var writeMu sync.Mutex
	sendFrame := func(pppFrame []byte) {
		rec := sslvpn.EncodeFrame(pppFrame)
		writeMu.Lock()
		_, _ = conn.Write(rec)
		writeMu.Unlock()
	}

	g.mu.Lock()
	dropEcho := g.dropEcho
	g.mu.Unlock()

	var srv *ppp.ServerSession
	srv = ppp.NewServerSession(ppp.ServerConfig{
		ClientIP: g.cfg.AssignedIP,
		ServerIP: g.cfg.ServerIP,
		DNS:      g.cfg.IPCPDNS,
		MRU:      ppp.DefaultMRU,
		DropEcho: dropEcho,
	}, ppp.ServerEvents{
		Send: sendFrame,
	})

	active := &activeTunnel{conn: conn, srv: srv}
	g.mu.Lock()
	g.tunnels[active] = struct{}{}
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.tunnels, active)
		g.mu.Unlock()
		_ = conn.Close()
	}()

	srv.Start()

	reader := io.Reader(conn)
	if buf != nil && buf.Reader.Buffered() > 0 {
		reader = io.MultiReader(buf.Reader, conn)
	}

	for {
		pppFrame, err := sslvpn.ReadFrame(reader)
		if err != nil {
			return
		}
		if ipPkt, ok := ppp.ExtractIPv4(pppFrame); ok {
			reflected := reflectIPv4(ipPkt)
			sendFrame(ppp.EncapsulateIP(reflected))
			continue
		}
		srv.Input(pppFrame)
	}
}

func (g *Gateway) handleLogout(w http.ResponseWriter, r *http.Request) {
	ck, err := r.Cookie(sslvpn.CookieName)
	if err == nil && ck.Value != "" {
		g.mu.Lock()
		delete(g.sessions, ck.Value)
		g.mu.Unlock()
	}
	g.logoutCount.Add(1)
	w.WriteHeader(http.StatusOK)
}

func (g *Gateway) validCookie(r *http.Request) bool {
	ck, err := r.Cookie(sslvpn.CookieName)
	if err != nil || ck.Value == "" {
		return false
	}
	g.mu.Lock()
	_, ok := g.sessions[ck.Value]
	g.mu.Unlock()
	return ok
}

// reflectIPv4 swaps the source and destination IPv4 addresses (and converts
// ICMP Echo Request type 8 into Echo Reply type 0) so data-plane tests can
// verify end-to-end packet reflection without a host network stack.
func reflectIPv4(pkt []byte) []byte {
	out := make([]byte, len(pkt))
	copy(out, pkt)
	ihl := int(out[0]&0x0f) * 4
	if len(out) < 20 || ihl < 20 || len(out) < ihl {
		return out
	}
	// Swap src (12..16) and dst (16..20).
	for i := 0; i < 4; i++ {
		out[12+i], out[16+i] = out[16+i], out[12+i]
	}
	// Recompute IPv4 header checksum.
	out[10], out[11] = 0, 0
	csum := ipv4Checksum(out[:ihl])
	binary.BigEndian.PutUint16(out[10:12], csum)

	// If ICMP Echo Request (proto 1, type 8), turn into Echo Reply (type 0).
	if out[9] == 1 && len(out) >= ihl+8 && out[ihl] == 8 {
		out[ihl] = 0
		out[ihl+2], out[ihl+3] = 0, 0
		icmpSum := ipv4Checksum(out[ihl:])
		binary.BigEndian.PutUint16(out[ihl+2:ihl+4], icmpSum)
	}
	return out
}

func ipv4Checksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(b[i : i+2]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
