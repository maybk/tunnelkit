package engine

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/fortivpn/fortitest"
)

func TestSAMLContextClosesPromptBeforeTunnelBecomesReady(t *testing.T) {
	gw := fortitest.NewGateway(fortitest.GatewayConfig{SAMLToken: "synthetic-saml"})
	defer gw.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := ln.Addr().String()
	_ = ln.Close()
	promptDone := make(chan struct{})
	handler := newTestHandler(func(ctx context.Context, p fortivpn.Prompt) (string, error) {
		resp, err := http.Get("http://" + address + "/?id=synthetic-saml")
		if err != nil {
			return "", err
		}
		_ = resp.Body.Close()
		<-ctx.Done()
		close(promptDone)
		return "", ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, fortivpn.Config{Host: gw.Host(), Port: gw.Port(), Auth: fortivpn.AuthSAML, TrustedCerts: []string{gw.CertFingerprint()}}, handler,
			WithDevice(fortitest.NewMemDevice("synthetic-saml", 1400)), WithSAMLListenAddr(address))
	}()
	select {
	case <-handler.connectedCh:
	case <-ctx.Done():
		t.Fatal("SAML did not complete")
	}
	select {
	case <-promptDone:
	default:
		t.Error("tunnel became ready while challenge was still pending")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("engine did not stop")
	}
}
