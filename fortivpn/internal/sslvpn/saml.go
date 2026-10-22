package sslvpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"tunnelkit/fortivpn"
)

// DefaultSAMLCallbackAddr is the localhost callback address FortiGate redirects
// external-browser SAML logins to.
const DefaultSAMLCallbackAddr = "127.0.0.1:8020"

// AuthenticateSAML runs the external-browser SAML flow:
//  1. Starts a local HTTP callback listener on 127.0.0.1:8020 (or c.SAMLListenAddr).
//  2. Emits a Prompt{Kind: PromptSAML, URL: base + "/remote/saml/start?redirect=1"} via promptFn.
//  3. Accepts the session id either from the browser localhost callback (/?id=...)
//     or directly from the value returned by promptFn.
//  4. Exchanges the session id at /remote/saml/auth_id?id=... for SVPNCOOKIE.
func (c *Client) AuthenticateSAML(ctx context.Context, promptFn func(ctx context.Context, p fortivpn.Prompt) (string, error)) (string, error) {
	if promptFn == nil {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, "SAML login requires interactive prompt handler")
	}

	listenAddr := c.SAMLListenAddr
	if listenAddr == "" {
		listenAddr = DefaultSAMLCallbackAddr
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fortivpn.WrapError(fortivpn.CodeAuthFailed, fmt.Sprintf("failed to bind SAML callback listener on %s", listenAddr), err)
	}
	defer ln.Close()

	idCh := make(chan string, 2)
	var once sync.Once
	deliverID := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" {
			once.Do(func() { idCh <- id })
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Connection", "close")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<html><body><h3>Missing SAML session ID.</h3></body></html>"))
			return
		}
		_, _ = w.Write([]byte("<html><body><h3>Authentication complete. You may close this browser window now.</h3></body></html>"))
		deliverID(id)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	promptCtx, cancelPrompt := context.WithCancel(ctx)
	defer cancelPrompt()

	promptErrCh := make(chan error, 1)
	go func() {
		val, err := promptFn(promptCtx, fortivpn.Prompt{
			ID:      "saml-login",
			Kind:    fortivpn.PromptSAML,
			Message: "Complete SAML Single Sign-On in your browser",
			URL:     c.baseURL() + PathSAMLStart,
		})
		if err != nil {
			promptErrCh <- err
			return
		}
		if val != "" {
			deliverID(val)
		}
	}()

	var samlID string
	select {
	case samlID = <-idCh:
		cancelPrompt()
	case err := <-promptErrCh:
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fortivpn.WrapError(fortivpn.CodePromptTimeout, "SAML login timed out", err)
		}
		return "", fortivpn.WrapError(fortivpn.CodeAuthFailed, "SAML login cancelled", err)
	case <-ctx.Done():
		return "", ctx.Err()
	}

	reqURL := c.baseURL() + PathSAMLAuthID + "?id=" + samlID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fortivpn.WrapError(fortivpn.CodeAuthFailed, "failed to create SAML auth_id request", err)
	}
	c.setCommonHeaders(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", c.classifyTransportError(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, fmt.Sprintf("SAML auth_id rejected with HTTP %d", resp.StatusCode))
	}
	cookie := c.extractCookie(resp)
	if cookie == "" {
		return "", fortivpn.NewError(fortivpn.CodeAuthFailed, "gateway did not issue SVPNCOOKIE after SAML authentication")
	}
	return cookie, nil
}
