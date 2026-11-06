package helperclient

import (
	"os"
	"path/filepath"
	"testing"

	"tunnelkit/fortivpn"
)

func TestCertificateMaterialIsPreparedWithoutMutatingProfilePaths(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "certificate.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, []byte("synthetic certificate material"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("synthetic test key material"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := fortivpn.Config{Host: "vpn.example.com", Username: "test", Password: fortivpn.NewSecret("synthetic-session-password"), ClientCert: &fortivpn.ClientCert{CertPath: certPath, KeyPath: keyPath}}
	cfg, err := prepareClientCertificate(original)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientCert.CertPath != "" || cfg.ClientCert.KeyPath != "" || cfg.ClientCert.CertPEM.IsEmpty() || cfg.ClientCert.KeyPEM.IsEmpty() {
		t.Fatal("IPC configuration must carry memory material only")
	}
	if original.ClientCert.CertPath != certPath || original.ClientCert.KeyPath != keyPath {
		t.Fatal("preparation mutated persisted profile paths")
	}
}
