package sslvpn

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"

	"tunnelkit/fortivpn"
)

// BuildTLSConfig constructs a *tls.Config that verifies the gateway's certificate
// either via the system CA pool + hostname verification OR via SHA-256 leaf DER
// fingerprint pinning (TrustedCerts). It never skips verification unconditionally.
func BuildTLSConfig(host string, trustedCerts []string, clientCert *fortivpn.ClientCert, legacyTLS bool, rootCAs *x509.CertPool) (*tls.Config, error) {
	pins := make(map[string]struct{}, len(trustedCerts))
	for _, raw := range trustedCerts {
		d := strings.ToLower(strings.TrimSpace(raw))
		if d != "" {
			pins[d] = struct{}{}
		}
	}

	var certs []tls.Certificate
	if clientCert != nil && clientCert.CertPath != "" && clientCert.KeyPath != "" {
		kp, err := tls.LoadX509KeyPair(clientCert.CertPath, clientCert.KeyPath)
		if err != nil {
			return nil, fortivpn.WrapError(fortivpn.CodeInvalidConfig, "failed to load client certificate", err)
		}
		certs = append(certs, kp)
	}

	minVersion := uint16(tls.VersionTLS12)
	var cipherSuites []uint16
	if legacyTLS {
		minVersion = tls.VersionTLS10
		for _, cs := range tls.CipherSuites() {
			cipherSuites = append(cipherSuites, cs.ID)
		}
		for _, cs := range tls.InsecureCipherSuites() {
			cipherSuites = append(cipherSuites, cs.ID)
		}
	}

	tlsCfg := &tls.Config{
		ServerName:         host,
		MinVersion:         minVersion,
		CipherSuites:       cipherSuites,
		Certificates:       certs,
		InsecureSkipVerify: true, // Custom VerifyPeerCertificate performs full verification.
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return verifyServerCert(host, rawCerts, pins, rootCAs)
		},
	}
	return tlsCfg, nil
}

func verifyServerCert(host string, rawCerts [][]byte, pins map[string]struct{}, rootCAs *x509.CertPool) error {
	if len(rawCerts) == 0 {
		return fortivpn.NewError(fortivpn.CodeCertUntrusted, "gateway presented no TLS certificate")
	}

	sum := sha256.Sum256(rawCerts[0])
	leafDigest := hex.EncodeToString(sum[:])

	// 1. Check explicit SHA-256 fingerprint pin first (matches openfortivpn trusted-cert).
	if _, pinned := pins[leafDigest]; pinned {
		return nil
	}

	// 2. Otherwise verify against system (or injected test) Root CAs + hostname/IP.
	certs := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		c, err := x509.ParseCertificate(raw)
		if err != nil {
			return &fortivpn.Error{
				Code:        fortivpn.CodeCertUntrusted,
				Detail:      fmt.Sprintf("failed to parse gateway certificate: %v", err),
				Fingerprint: leafDigest,
				Cause:       err,
			}
		}
		certs = append(certs, c)
	}

	intermediates := x509.NewCertPool()
	for _, ic := range certs[1:] {
		intermediates.AddCert(ic)
	}

	opts := x509.VerifyOptions{
		DNSName:       host,
		Roots:         rootCAs,
		Intermediates: intermediates,
	}
	if _, err := certs[0].Verify(opts); err != nil {
		return &fortivpn.Error{
			Code:        fortivpn.CodeCertUntrusted,
			Detail:      fmt.Sprintf("gateway certificate is not trusted (sha256: %s): %v", leafDigest, err),
			Fingerprint: leafDigest,
			Cause:       err,
		}
	}
	return nil
}

// CertFingerprint returns the lowercase 64-character hex SHA-256 digest of a DER certificate.
func CertFingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}
