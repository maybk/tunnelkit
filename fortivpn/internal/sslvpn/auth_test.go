package sslvpn_test

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/internal/sslvpn"
)

func newAuthenticationTestClient(t *testing.T, handler http.Handler) *sslvpn.Client {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	host, ps, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(ps)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	c, err := sslvpn.NewClient(fortivpn.Config{Host: host, Port: port, Username: "synthetic-user", Password: "synthetic-password"}, nil, sslvpn.WithRootCAs(pool))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSuccessfulLoginWithMetadataDoesNotRequestAnotherChallenge(t *testing.T) {
	c := newAuthenticationTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sslvpn.CookieName, Value: "synthetic-session", Path: "/"})
		_, _ = w.Write([]byte("ret=1,tokeninfo=totp,chal_msg=Synthetic token description"))
	}))
	cookie, err := c.Login(context.Background(), func(context.Context, fortivpn.Prompt) (string, error) {
		t.Error("successful authentication reopened a challenge")
		return "", context.Canceled
	})
	if err != nil || cookie == "" {
		t.Fatalf("successful authentication failed: %v", err)
	}
}

func TestLoginCanPromptWhenGatewayUsesRetOneWithoutSession(t *testing.T) {
	calls := 0
	c := newAuthenticationTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte("ret=1,tokeninfo=totp,reqid=synthetic,chal_msg=Synthetic challenge"))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sslvpn.CookieName, Value: "synthetic-session", Path: "/"})
		_, _ = w.Write([]byte("ret=1"))
	}))
	prompted := false
	_, err := c.Login(context.Background(), func(context.Context, fortivpn.Prompt) (string, error) {
		prompted = true
		return "synthetic-answer", nil
	})
	if err != nil || !prompted {
		t.Fatalf("gateway challenge was not handled: %v", err)
	}
}

func TestLoginAcceptsSessionIssuedBeforePortalRedirect(t *testing.T) {
	c := newAuthenticationTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sslvpn.PathLoginCheck {
			http.SetCookie(w, &http.Cookie{Name: sslvpn.CookieName, Value: "synthetic-session", Path: "/"})
			http.Redirect(w, r, "/synthetic-portal", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("<html><body>Synthetic portal</body></html>"))
	}))
	if cookie, err := c.Login(context.Background(), nil); err != nil || cookie == "" {
		t.Fatalf("redirect login failed: %v", err)
	}
}

func TestSAMLCompletionWaitsForPromptCleanup(t *testing.T) {
	exchanged := make(chan struct{}, 1)
	c := newAuthenticationTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanged <- struct{}{}
		http.SetCookie(w, &http.Cookie{Name: sslvpn.CookieName, Value: "synthetic-session", Path: "/"})
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c.SAMLListenAddr = listener.Addr().String()
	_ = listener.Close()
	cleaning, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseCleanup := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseCleanup()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := c.AuthenticateSAML(ctx, func(pCtx context.Context, p fortivpn.Prompt) (string, error) {
			resp, err := http.Get("http://" + c.SAMLListenAddr + "/?id=synthetic-session")
			if err != nil {
				return "", err
			}
			_ = resp.Body.Close()
			<-pCtx.Done()
			close(cleaning)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return "", pCtx.Err()
		})
		done <- err
	}()
	select {
	case <-cleaning:
	case <-ctx.Done():
		t.Fatal("prompt context was not cancelled")
	}
	select {
	case <-exchanged:
		t.Fatal("authentication continued before prompt cleanup")
	case <-time.After(20 * time.Millisecond):
	}
	releaseCleanup()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SAML completion failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SAML cleanup did not finish")
	}
}
