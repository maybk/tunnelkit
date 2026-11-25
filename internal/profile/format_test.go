package profile

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"tunnelkit/internal/credentials"
	"tunnelkit/internal/storage"
)

func TestLegacyDocumentsRemainReadableAndBecomeVersionedOnSave(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	p, err := store.Save(Input{Name: "migration", Host: "vpn.example.com", Username: "u", Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	// Model the two unversioned files produced by the released application.
	for _, path := range []string{storage.ProfilesPath(root), storage.SecretsPath(root)} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data jsontext.Value `json:"data"`
		}
		_ = json.Unmarshal(raw, &envelope)
		if len(envelope.Data) > 0 {
			raw = envelope.Data
		}
		if err := storage.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, pwd, err := store.GetWithPassword(p.ID); err != nil || pwd != "synthetic-password" {
		t.Fatal("legacy credentials lost", err)
	}
	if _, err := store.Save(Input{ID: p.ID, Name: "updated", Host: p.Host, Username: p.Username, Password: "synthetic-replacement"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{storage.ProfilesPath(root), storage.SecretsPath(root)} {
		raw, _ := os.ReadFile(path)
		var h struct {
			Version int `json:"schemaVersion"`
		}
		_ = json.Unmarshal(raw, &h)
		if h.Version != 1 {
			t.Fatal("missing file schema version", path)
		}
	}
}

func TestFutureProfileAndCredentialFormatsArePreserved(t *testing.T) {
	for _, file := range []string{"profiles", "secrets"} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)
			p, err := store.Save(Input{Name: "migration", Host: "vpn.example.com", Username: "u", Password: "synthetic-password"})
			if err != nil {
				t.Fatal(err)
			}
			path := storage.ProfilesPath(root)
			if file == "secrets" {
				path = storage.SecretsPath(root)
			}
			raw := []byte(`{"schemaVersion":99,"data":{}}`)
			if err := storage.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Save(Input{ID: p.ID, Name: p.Name, Host: p.Host, Username: p.Username, Password: "synthetic-replacement"}); err == nil {
				t.Fatal("future format accepted")
			}
			if _, err := store.List(); err == nil {
				t.Fatal("future format was hidden from the caller")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("future document overwritten")
			}
		})
	}
}

func TestEmptyProfileAndSecretFilesDefaultToEmptyCollections(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{storage.ProfilesPath(root), storage.SecretsPath(root)} {
		if err := storage.WriteFile(path, []byte(" \n\t "), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := NewStore(root)
	list, err := store.List()
	if err != nil || len(list) != 0 {
		t.Fatalf("List() on whitespace files = (%v, %v), want empty slice", list, err)
	}
	saved, err := store.Save(Input{Name: "empty-init", Host: "vpn.example.com", Username: "u", Password: "p"})
	if err != nil || !saved.HasPassword {
		t.Fatalf("Save() after whitespace files = (%+v, %v)", saved, err)
	}
}

func TestStoreRejectsSymlinkProfileAndSecretFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	for _, tc := range []struct {
		name string
		path func(string) string
	}{
		{name: "profiles", path: storage.ProfilesPath},
		{name: "secrets", path: storage.SecretsPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)
			if _, err := store.Save(Input{Name: "symlink-check", Host: "vpn.example.com", Username: "u", Password: "p"}); err != nil {
				t.Fatalf("Save: %v", err)
			}
			target := tc.path(root)
			moved := filepath.Join(root, tc.name+".real.json")
			if err := os.Rename(target, moved); err != nil {
				t.Fatalf("Rename: %v", err)
			}
			if err := os.Symlink(moved, target); err != nil {
				t.Fatalf("Symlink: %v", err)
			}
			if _, err := store.List(); err == nil {
				t.Fatalf("expected List to reject symlinked %s file", tc.name)
			}
		})
	}
}

func TestStoreSupportsUTF8BOMFiles(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	saved, err := store.Save(Input{Name: "bom-vpn", Host: "vpn.example.com", Username: "u", Password: "bom-password"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	bom := []byte{0xEF, 0xBB, 0xBF}
	for _, path := range []string{storage.ProfilesPath(root), storage.SecretsPath(root), storage.SecretKeyPath(root)} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		if err := storage.WritePrivateFile(path, append(append([]byte(nil), bom...), raw...)); err != nil {
			t.Fatalf("WritePrivateFile BOM %s: %v", path, err)
		}
	}

	reloaded := NewStore(root)
	list, err := reloaded.List()
	if err != nil || len(list) != 1 || !list[0].HasPassword {
		t.Fatalf("List with BOM = (%+v, %v)", list, err)
	}
	_, pwd, err := reloaded.GetWithPassword(saved.ID)
	if err != nil || pwd != "bom-password" {
		t.Fatalf("GetWithPassword with BOM = (%q, %v)", pwd, err)
	}
}

func TestStoreGuardsAgainstMixedMasterKeysAndFormatsErrors(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	first, err := store.Save(Input{Name: "Office A", Host: "a.example.com", Username: "u1", Password: "pwd-a"})
	if err != nil {
		t.Fatalf("Save first: %v", err)
	}
	second, err := store.Save(Input{Name: "Office B", Host: "b.example.com", Username: "u2", Password: "pwd-b"})
	if err != nil {
		t.Fatalf("Save second: %v", err)
	}

	secretsBefore, err := os.ReadFile(storage.SecretsPath(root))
	if err != nil {
		t.Fatalf("ReadFile secrets.json: %v", err)
	}
	if err := os.Remove(storage.SecretKeyPath(root)); err != nil {
		t.Fatalf("Remove secret.key: %v", err)
	}

	// 读取已有配置密码时应明确指出缺少 secret.key，且绝不自动新建密钥。
	if _, _, err := store.GetWithPassword(first.ID); !errors.Is(err, credentials.ErrMasterKeyMissing) {
		t.Fatalf("GetWithPassword missing key err = %v, want ErrMasterKeyMissing", err)
	} else if !strings.Contains(err.Error(), "secret.key") {
		t.Fatalf("expected actionable error message mentioning secret.key, got %q", err.Error())
	}
	if _, err := os.Stat(storage.SecretKeyPath(root)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("GetWithPassword must not recreate secret.key, stat err=%v", err)
	}

	// 存在其他已保存密码时，新增或修改密码必须被阻止以防双密钥混写。
	if _, err := store.Save(Input{Name: "Office C", Host: "c.example.com", Username: "u3", Password: "pwd-c"}); !errors.Is(err, credentials.ErrMasterKeyMissing) {
		t.Fatalf("Save new profile with missing key err = %v, want ErrMasterKeyMissing", err)
	}
	if _, err := store.Save(Input{ID: second.ID, Name: second.Name, Host: second.Host, Username: second.Username, Password: "pwd-b2"}); !errors.Is(err, credentials.ErrMasterKeyMissing) {
		t.Fatalf("Update one of multiple secrets with missing key err = %v, want ErrMasterKeyMissing", err)
	}
	if _, err := os.Stat(storage.SecretKeyPath(root)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("guarded Save must not recreate secret.key, stat err=%v", err)
	}
	secretsAfter, err := os.ReadFile(storage.SecretsPath(root))
	if err != nil || !bytes.Equal(secretsBefore, secretsAfter) {
		t.Fatalf("guarded Save mutated secrets.json")
	}

	// 写入未迁移的旧版 Windows DPAPI 魔数头密钥，验证给出清晰迁移提示。
	dpapiHeader := []byte{
		0x01, 0x00, 0x00, 0x00,
		0xD0, 0x8C, 0x9D, 0xDF, 0x01, 0x15, 0xD1, 0x11,
		0x8C, 0x7A, 0x00, 0xC0, 0x4F, 0xC2, 0x97, 0xEB,
	}
	syntheticDPAPI := append(dpapiHeader, bytes.Repeat([]byte{0x42}, 64)...)
	if err := storage.WritePrivateFile(storage.SecretKeyPath(root), []byte(base64.StdEncoding.EncodeToString(syntheticDPAPI)+"\n")); err != nil {
		t.Fatalf("write synthetic DPAPI key: %v", err)
	}
	if _, _, err := store.GetWithPassword(first.ID); !errors.Is(err, credentials.ErrLegacyKeyMigrationRequired) {
		t.Fatalf("GetWithPassword synthetic DPAPI err = %v, want ErrLegacyKeyMigrationRequired", err)
	} else if !strings.Contains(err.Error(), "原 Windows 设备") {
		t.Fatalf("expected Windows migration guidance, got %q", err.Error())
	}

	// 删除其余配置仅保留唯一条目且删除密钥后，允许重置唯一条目并重建密钥。
	if err := os.Remove(storage.SecretKeyPath(root)); err != nil {
		t.Fatalf("Remove synthetic secret.key: %v", err)
	}
	if err := store.Delete(second.ID); err != nil {
		t.Fatalf("Delete second: %v", err)
	}
	if _, err := store.Save(Input{ID: first.ID, Name: first.Name, Host: first.Host, Username: first.Username, Password: "pwd-a-reset"}); err != nil {
		t.Fatalf("expected overwriting sole remaining secret without key to succeed: %v", err)
	}
	if _, pwd, err := store.GetWithPassword(first.ID); err != nil || pwd != "pwd-a-reset" {
		t.Fatalf("GetWithPassword after sole secret reset = (%q, %v)", pwd, err)
	}
}

func TestStorePrepareStorageNoOpAndTightensPermissions(t *testing.T) {
	emptyRoot := t.TempDir()
	NewStore(emptyRoot).PrepareStorage()
	for _, path := range []string{storage.ProfilesPath(emptyRoot), storage.SecretsPath(emptyRoot), storage.SecretKeyPath(emptyRoot)} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("PrepareStorage must not create %s in empty dir, stat err=%v", path, err)
		}
	}

	if runtime.GOOS != "windows" {
		root := t.TempDir()
		store := NewStore(root)
		if _, err := store.Save(Input{Name: "perm", Host: "vpn.example.com", Username: "u", Password: "p"}); err != nil {
			t.Fatal(err)
		}
		files := []string{storage.ProfilesPath(root), storage.SecretsPath(root), storage.SecretKeyPath(root)}
		for _, p := range files {
			if err := os.Chmod(p, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		store.PrepareStorage()
		for _, p := range files {
			if storage.HasBroadPermissions(p) {
				t.Fatalf("expected PrepareStorage to tighten permissions on %s", p)
			}
		}
	}
}
