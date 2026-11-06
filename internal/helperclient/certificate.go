package helperclient

import (
	"fmt"
	"io"

	"tunnelkit/fortivpn"
)

// Read certificate files with desktop permissions before requesting a privileged
// session. Only in-memory credentials cross IPC; the helper never opens user paths.
func prepareClientCertificate(cfg fortivpn.Config) (fortivpn.Config, error) {
	if cfg.ClientCert == nil {
		return cfg, nil
	}
	cert := *cfg.ClientCert
	if cert.CertPath == "" && cert.KeyPath == "" {
		return cfg, cfg.Validate()
	}
	read := func(path string) (fortivpn.Secret, error) {
		f, err := openCertificateFile(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("客户端证书必须是普通文件")
		}
		data, err := io.ReadAll(io.LimitReader(f, (128<<10)+1))
		if err != nil {
			return "", err
		}
		if len(data) > 128<<10 {
			return "", fmt.Errorf("客户端证书或私钥超过 128 KiB")
		}
		return fortivpn.NewSecret(string(data)), nil
	}
	var err error
	cert.CertPEM, err = read(cert.CertPath)
	if err != nil {
		return cfg, fmt.Errorf("读取客户端证书失败: %w", err)
	}
	cert.KeyPEM, err = read(cert.KeyPath)
	if err != nil {
		return cfg, fmt.Errorf("读取客户端私钥失败: %w", err)
	}
	cert.CertPath, cert.KeyPath = "", ""
	cfg.ClientCert = &cert
	return cfg, cfg.Validate()
}
