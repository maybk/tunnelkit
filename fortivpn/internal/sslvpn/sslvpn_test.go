package sslvpn_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/fortitest"
	"tunnelkit/fortivpn/internal/sslvpn"
)

func TestFrameLayoutAndRoundTrip(t *testing.T) {
	ppp := []byte{0xff, 0x03, 0xc0, 0x21, 0x01}
	got := sslvpn.EncodeFrame(ppp)
	want := []byte{
		0x00, 0x0b,
		0x50, 0x50,
		0x00, 0x05,
		0xff, 0x03, 0xc0, 0x21, 0x01,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFrame = % x, want % x", got, want)
	}

	parsed, rest, err := sslvpn.ParseFrame(got)
	if err != nil || !bytes.Equal(parsed, ppp) || len(rest) != 0 {
		t.Fatalf("ParseFrame = %x, rest=%d, err=%v", parsed, len(rest), err)
	}
}

func TestParseLoginResultPreservesCommasInChalMsg(t *testing.T) {
	raw := "ret=2,tokeninfo=totp,reqid=101,polid=1,grp=vpn,chal_msg=Hello, please enter your 6-digit code, now,magic=m99"
	res, err := sslvpn.ParseLoginResult(raw)
	if err != nil {
		t.Fatalf("ParseLoginResult: %v", err)
	}
	if res.Ret != 2 || !res.IsChallenge() {
		t.Fatalf("unexpected LoginResult: %+v", res)
	}
	chal := res.Challenge()
	wantMsg := "Hello, please enter your 6-digit code, now"
	if chal.Message != wantMsg {
		t.Fatalf("Challenge.Message = %q, want %q", chal.Message, wantMsg)
	}
	if chal.Echo["magic"] != "m99" || chal.Echo["reqid"] != "101" {
		t.Fatalf("Challenge.Echo = %+v", chal.Echo)
	}
}

func TestChallengeFormMagicOrderAndFTMPush(t *testing.T) {
	c := sslvpn.Challenge{
		TokenInfo: "ftm_push",
		Echo: map[string]string{
			"magic": "m1", "reqid": "r1", "polid": "p1", "grp": "g1", "portal": "P1", "peer": "e1",
		},
	}
	bodyWithCode := sslvpn.BuildChallengeForm("alice", "123456", "", c)
	idx := strings.Index(bodyWithCode, "&magic=")
	if idx < 0 || strings.Contains(bodyWithCode[idx+len("&magic="):], "&") {
		t.Fatalf("magic must be last in coded challenge form: %q", bodyWithCode)
	}

	bodyPush := sslvpn.BuildChallengeForm("alice", "", "", c)
	if strings.Contains(bodyPush, "magic=") || !strings.HasSuffix(bodyPush, "&ftmpush=1") {
		t.Fatalf("FTM push form must drop magic and end with &ftmpush=1: %q", bodyPush)
	}
}

func TestParseConfigXMLDeduplicatesRoutesAndIgnoresZeroDNS(t *testing.T) {
	rawXML := `<?xml version="1.0" encoding="utf-8"?>
<sslvpn-tunnel ver="2" dtls="1">
  <fos platform="FG200E" major="7" minor="2" patch="8"/>
  <auth-ses check-src-ip="1" tun-connect-without-reauth="1"/>
  <ipv4>
    <dns ip="0.0.0.0"/>
    <dns ip="10.20.4.10" domain="corp.example.com"/>
    <dns ip="10.20.4.10"/>
    <split-dns domains="int.example.com,corp.example.com" dnsserver1="10.20.4.11" dnsserver2="0.0.0.0"/>
    <assigned-addr ipv4="10.212.0.10"/>
    <split-tunnel-info>
      <addr ip="172.16.10.2" mask="255.255.255.255"/>
      <addr ip="172.16.10.2" mask="255.255.255.255"/>
      <addr ip="172.16.10.1" mask="255.255.255.255"/>
    </split-tunnel-info>
    <split-tunnel-info negate="1">
      <addr ip="192.168.1.0" mask="255.255.255.0"/>
    </split-tunnel-info>
  </ipv4>
  <idle-timeout val="3600"/>
  <auth-timeout val="18000"/>
</sslvpn-tunnel>`

	cfg, err := sslvpn.ParseConfigXML([]byte(rawXML))
	if err != nil {
		t.Fatalf("ParseConfigXML: %v", err)
	}
	if cfg.AssignedIP != netip.MustParseAddr("10.212.0.10") {
		t.Fatalf("AssignedIP = %v, want 10.212.0.10", cfg.AssignedIP)
	}
	if len(cfg.DNS) != 2 || cfg.DNS[0] != netip.MustParseAddr("10.20.4.10") || cfg.DNS[1] != netip.MustParseAddr("10.20.4.11") {
		t.Fatalf("DNS = %v, want [10.20.4.10, 10.20.4.11]", cfg.DNS)
	}
	if len(cfg.DNSDomains) != 2 || cfg.DNSDomains[0] != "corp.example.com" || cfg.DNSDomains[1] != "int.example.com" {
		t.Fatalf("DNSDomains = %v", cfg.DNSDomains)
	}
	if len(cfg.SplitRoutes) != 2 {
		t.Fatalf("SplitRoutes = %v, want 2 deduplicated routes", cfg.SplitRoutes)
	}
	if len(cfg.ExcludeRoutes) != 1 {
		t.Fatalf("ExcludeRoutes = %v, want 1 exclude route", cfg.ExcludeRoutes)
	}
	if !cfg.ReconnectWithoutReauth {
		t.Fatal("expected ReconnectWithoutReauth = true")
	}
	if cfg.FortiOS != "FG200E v7.2.8" {
		t.Fatalf("FortiOS = %q, want FG200E v7.2.8", cfg.FortiOS)
	}
}

func TestParseConfigXMLLegacyFlatAndDNSSuffixAndRejectsIPDomain(t *testing.T) {
	rawXML := `<?xml version="1.0" encoding="utf-8"?>
<sslvpn-tunnel ver="1">
  <assigned-addr ipv4="10.10.1.5"/>
  <dns ip="10.10.1.1" domain="10.10.1.1" suffix="legacy.example.com"/>
  <dns-suffix val="corp.example.com; sub.example.com"/>
  <ipv4>
    <dns-suffix>inner.example.com</dns-suffix>
  </ipv4>
  <split-tunnel-info>
    <addr ip="10.20.0.0" mask="255.255.0.0"/>
    <addr ip="10.30.0.0" mask="16"/>
  </split-tunnel-info>
</sslvpn-tunnel>`

	cfg, err := sslvpn.ParseConfigXML([]byte(rawXML))
	if err != nil {
		t.Fatalf("ParseConfigXML: %v", err)
	}
	if cfg.AssignedIP != netip.MustParseAddr("10.10.1.5") {
		t.Fatalf("AssignedIP = %v, want 10.10.1.5", cfg.AssignedIP)
	}
	if len(cfg.DNSDomains) != 4 ||
		cfg.DNSDomains[0] != "legacy.example.com" ||
		cfg.DNSDomains[1] != "inner.example.com" ||
		cfg.DNSDomains[2] != "corp.example.com" ||
		cfg.DNSDomains[3] != "sub.example.com" {
		t.Fatalf("DNSDomains = %v, want [legacy.example.com inner.example.com corp.example.com sub.example.com] (with IP literal 10.10.1.1 rejected)", cfg.DNSDomains)
	}
	for _, d := range cfg.DNSDomains {
		if d == "10.10.1.1" {
			t.Fatalf("expected IP literal domain 10.10.1.1 to be rejected, got %v", cfg.DNSDomains)
		}
	}
	if len(cfg.SplitRoutes) != 2 ||
		cfg.SplitRoutes[0] != netip.MustParsePrefix("10.20.0.0/16") ||
		cfg.SplitRoutes[1] != netip.MustParsePrefix("10.30.0.0/16") {
		t.Fatalf("SplitRoutes = %v, want [10.20.0.0/16 10.30.0.0/16]", cfg.SplitRoutes)
	}
}

func TestTunnelUpgradeDetectsHTMLResponseAsCodeTunnelDenied(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		_, _ = c2.Write([]byte("HTTP/1.1 200 OK\r\n\r\n<html><body>Portal Only</body></html>"))
		_ = c2.Close()
	}()

	tc := sslvpn.NewTunnelConn(c1)
	_, err := tc.ReadFrame()
	if !fortivpn.IsCode(err, fortivpn.CodeTunnelDenied) {
		t.Fatalf("expected CodeTunnelDenied on HTML tunnel body, got %v", err)
	}
}

func TestControlPlaneFlowsAgainstFakeGateway(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{
		Users: map[string]string{
			"alice": "pw123",
			"bob":   "pw456",
			"carol": "pw789",
		},
		OTPCode: map[string]string{
			"bob": "654321",
		},
		FTMPush: map[string]bool{
			"carol": true,
		},
		SAMLToken:  "saml-session-xyz",
		AssignedIP: netip.MustParseAddr("10.212.0.10"),
		DNS:        []netip.Addr{netip.MustParseAddr("10.20.0.53")},
		DNSDomains: []string{"corp.internal"},
		SplitRoutes: []netip.Prefix{
			netip.MustParsePrefix("172.16.10.2/32"),
		},
		DuplicateRoutes: []netip.Prefix{
			netip.MustParsePrefix("172.16.10.2/32"),
		},
	})
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	untrustedClient, err := sslvpn.NewClient(fortivpn.Config{
		Host:     gw.Host(),
		Port:     gw.Port(),
		Username: "alice",
		Password: "pw123",
	}, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = untrustedClient.Login(ctx, nil)
	fe, ok := fortivpn.AsError(err)
	if !ok || fe.Code != fortivpn.CodeCertUntrusted || fe.Fingerprint != gw.CertFingerprint() {
		t.Fatalf("expected CodeCertUntrusted with fingerprint %s, got %+v", gw.CertFingerprint(), err)
	}

	pinnedClient, err := sslvpn.NewClient(fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Username:     "alice",
		Password:     "pw123",
		TrustedCerts: []string{gw.CertFingerprint()},
	}, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cookie, err := pinnedClient.Login(ctx, nil)
	if err != nil {
		t.Fatalf("Login with pinned cert: %v", err)
	}
	tcfg, err := pinnedClient.FetchTunnelConfig(ctx, cookie)
	if err != nil {
		t.Fatalf("FetchTunnelConfig: %v", err)
	}
	if len(tcfg.SplitRoutes) != 1 {
		t.Fatalf("SplitRoutes = %v, want 1 deduplicated route", tcfg.SplitRoutes)
	}
	if err := pinnedClient.Logout(ctx, cookie); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	otpClient, _ := sslvpn.NewClient(fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Username:     "bob",
		Password:     "pw456",
		TrustedCerts: []string{gw.CertFingerprint()},
	}, nil)
	_, err = otpClient.Login(ctx, func(_ context.Context, p fortivpn.Prompt) (string, error) {
		if p.Kind != fortivpn.PromptOTP {
			t.Fatalf("Prompt.Kind = %q, want otp", p.Kind)
		}
		return "654321", nil
	})
	if err != nil {
		t.Fatalf("OTP Login: %v", err)
	}

	pushClient, _ := sslvpn.NewClient(fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Username:     "carol",
		Password:     "pw789",
		TrustedCerts: []string{gw.CertFingerprint()},
	}, nil)
	_, err = pushClient.Login(ctx, func(_ context.Context, p fortivpn.Prompt) (string, error) {
		if p.Kind != fortivpn.PromptFTMPush {
			t.Fatalf("Prompt.Kind = %q, want ftm_push", p.Kind)
		}
		return "", nil
	})
	if err != nil {
		t.Fatalf("FTM Push Login: %v", err)
	}

	samlClient, _ := sslvpn.NewClient(fortivpn.Config{
		Host:         gw.Host(),
		Port:         gw.Port(),
		Auth:         fortivpn.AuthSAML,
		TrustedCerts: []string{gw.CertFingerprint()},
	}, nil, sslvpn.WithSAMLListenAddr("127.0.0.1:18020"))
	_, err = samlClient.Login(ctx, func(pCtx context.Context, p fortivpn.Prompt) (string, error) {
		if p.Kind != fortivpn.PromptSAML {
			t.Fatalf("Prompt.Kind = %q, want saml", p.Kind)
		}
		resp, err := http.Get("http://127.0.0.1:18020/?id=saml-session-xyz")
		if err != nil {
			return "", err
		}
		_ = resp.Body.Close()
		<-pCtx.Done()
		return "", pCtx.Err()
	})
	if err != nil {
		t.Fatalf("SAML Login: %v", err)
	}
}

func TestLoginAcceptsFortiOS6HTMLRedirectWithCookie(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sslvpn.PathLoginCheck {
			http.SetCookie(w, &http.Cookie{
				Name:  sslvpn.CookieName,
				Value: "fortios6-session-cookie",
				Path:  "/",
			})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><head>\n<script language=\"javascript\">\ndocument.location='/sslvpn/portal.html';\n</script>\n</head></html>\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host, portStr, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portStr)

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	c, err := sslvpn.NewClient(fortivpn.Config{
		Host:     host,
		Port:     port,
		Username: "alice",
		Password: "secret",
	}, nil, sslvpn.WithRootCAs(pool))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	cookie, err := c.Login(context.Background(), nil)
	if err != nil {
		t.Fatalf("Login failed on FortiOS 6 HTML redirect: %v", err)
	}
	if cookie != "fortios6-session-cookie" {
		t.Fatalf("cookie = %q, want fortios6-session-cookie", cookie)
	}
}

func TestFrameHotPathZeroAllocations(t *testing.T) {
	payload := []byte{0xff, 0x03, 0x00, 0x21, 0x45, 0x00, 0x00, 0x14}
	frame := sslvpn.EncodeFrame(payload)
	reader := bytes.NewReader(frame)
	dst := make([]byte, 64)
	var scratch [sslvpn.FrameHeaderLen]byte

	allocs := testing.AllocsPerRun(100, func() {
		reader.Reset(frame)
		_, _ = sslvpn.ReadFrameInto(reader, dst)
		sslvpn.PutFrameHeader(scratch[:], len(payload))
		_, _, _ = sslvpn.ParseFrame(frame)
	})
	if allocs > 0 {
		t.Fatalf("SSL-VPN frame hot path allocated %.2f times per run, want 0", allocs)
	}
}

func FuzzFrameAndConfigXML(f *testing.F) {
	f.Add([]byte{0x00, 0x0b, 0x50, 0x50, 0x00, 0x05, 0xff, 0x03, 0xc0, 0x21, 0x01})
	f.Add([]byte(`<?xml version="1.0"?><sslvpn-tunnel><ipv4><assigned-addr ipv4="10.0.0.1"/></ipv4></sslvpn-tunnel>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = sslvpn.ParseFrame(data)
		_, _ = sslvpn.ParseConfigXML(data)
		_, _ = sslvpn.ParseLoginResult(string(data))
	})
}
