package sslvpn

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"tunnelkit/fortivpn"
)

func TestBuildTLSConfigWithInMemoryClientCertificate(t *testing.T) {
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert := &fortivpn.ClientCert{CertPEM: fortivpn.NewSecret(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))), KeyPEM: fortivpn.NewSecret(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})))}
	cfg, err := BuildTLSConfig("vpn.example.com", nil, cert, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Certificates) != 1 || cfg.Certificates[0].PrivateKey == nil {
		t.Fatal("client authentication material was not loaded")
	}
}
