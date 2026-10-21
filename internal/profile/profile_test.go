package profile

import (
	"errors"
	"os"
	"strings"
	"testing"

	"tunnelkit/internal/storage"
)

func TestStoreCRUDAndPasswordPreservation(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	created, err := store.Save(Input{
		Name:        "公司 VPN",
		Host:        "vpn.example.com",
		Port:        0, // 应自动补默认 443
		Username:    "alice",
		Password:    "s3cr3t",
		Realm:       "corp",
		ExtraRoutes: []string{"10.10.0.0/16"},
		DNSDomains:  []string{"corp.example.com"},
		Reconnect:   true,
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if created.Port != DefaultPort || !created.HasPassword {
		t.Fatalf("unexpected created profile: %+v", created)
	}
	if created.Realm != "corp" || created.Routes != "gateway" || created.DNS != "split" || !created.Reconnect {
		t.Fatalf("unexpected created fields: %+v", created)
	}

	rawJSON, err := os.ReadFile(storage.ProfilesPath(root))
	if err != nil {
		t.Fatalf("read profiles.json failed: %v", err)
	}
	if strings.Contains(string(rawJSON), "s3cr3t") {
		t.Fatalf("profiles.json must not contain plaintext password")
	}

	// 编辑时不传密码，应保留原密码。
	updated, err := store.Save(Input{
		ID:       created.ID,
		Name:     "公司主网关",
		Host:     "vpn2.example.com",
		Port:     8443,
		Username: "alice",
		Password: "",
	})
	if err != nil {
		t.Fatalf("update without password failed: %v", err)
	}
	if updated.Name != "公司主网关" || updated.Port != 8443 {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}

	_, pwd, err := store.GetWithPassword(created.ID)
	if err != nil {
		t.Fatalf("GetWithPassword failed: %v", err)
	}
	if pwd != "s3cr3t" {
		t.Fatalf("expected preserved password 's3cr3t', got %q", pwd)
	}

	// 信任证书
	digest := strings.Repeat("a1", 32)
	withCert, err := store.TrustCert(created.ID, digest)
	if err != nil {
		t.Fatalf("TrustCert failed: %v", err)
	}
	if withCert.TrustedCert != digest {
		t.Fatalf("expected cert %q, got %q", digest, withCert.TrustedCert)
	}

	// 删除配置应同步删除密码。
	if err := store.Delete(created.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, _, err := store.GetWithPassword(created.ID); err == nil {
		t.Fatalf("expected error after deleting profile")
	}
}

func TestSaveRejectsUnknownID(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Save(Input{
		ID:       strings.Repeat("a", 32),
		Name:     "A",
		Host:     "vpn.example.com",
		Username: "u",
		Password: "p",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown ID, got %v", err)
	}
}

func TestValidationRules(t *testing.T) {
	cases := []struct {
		name string
		in   Input
	}{
		{
			name: "missing password on new profile",
			in:   Input{Name: "A", Host: "vpn.example.com", Username: "u"},
		},
		{
			name: "invalid port",
			in:   Input{Name: "A", Host: "vpn.example.com", Port: 70000, Username: "u", Password: "p"},
		},
		{
			name: "invalid cert digest",
			in:   Input{Name: "A", Host: "vpn.example.com", Username: "u", Password: "p", TrustedCert: "1234"},
		},
		{
			name: "default route in extraRoutes",
			in:   Input{Name: "A", Host: "vpn.example.com", Username: "u", Password: "p", ExtraRoutes: []string{"0.0.0.0/0"}},
		},
		{
			name: "malformed CIDR in extraRoutes",
			in:   Input{Name: "A", Host: "vpn.example.com", Username: "u", Password: "p", ExtraRoutes: []string{"not-a-cidr"}},
		},
		{
			name: "incomplete clientCert",
			in:   Input{Name: "A", Host: "vpn.example.com", Username: "u", Password: "p", ClientCert: "/path/cert.pem"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeAndValidate(tc.in, false); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}
