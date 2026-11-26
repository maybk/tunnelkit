package profile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"tunnelkit/internal/storage"
)

func TestStoreSettingsDefaultsAndPatchPersistence(t *testing.T) {
	rootDir := t.TempDir()
	store := NewStore(rootDir)

	got, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings on missing file failed: %v", err)
	}
	if got != DefaultSettings() {
		t.Fatalf("GetSettings() = %+v, want %+v", got, DefaultSettings())
	}

	dark := "dark"
	saved, err := store.PatchSettings(SettingsPatch{Theme: &dark})
	if err != nil {
		t.Fatalf("PatchSettings(theme) failed: %v", err)
	}
	if saved.Theme != "dark" || !saved.AutoCheckUpdate {
		t.Fatalf("unexpected saved settings after theme patch: %+v", saved)
	}

	disabled := false
	saved, err = store.PatchSettings(SettingsPatch{AutoCheckUpdate: &disabled})
	if err != nil {
		t.Fatalf("PatchSettings(autoCheckUpdate) failed: %v", err)
	}
	if saved.Theme != "dark" || saved.AutoCheckUpdate {
		t.Fatalf("unexpected saved settings after autoCheckUpdate patch: %+v", saved)
	}

	reloaded, err := NewStore(rootDir).GetSettings()
	if err != nil {
		t.Fatalf("reloaded GetSettings failed: %v", err)
	}
	if reloaded != saved {
		t.Fatalf("reloaded settings = %+v, want %+v", reloaded, saved)
	}
}

func TestStoreSettingsNormalizesInvalidThemeAndPreservesMissingFields(t *testing.T) {
	rootDir := t.TempDir()
	store := NewStore(rootDir)

	unknown := "neon-pink"
	saved, err := store.PatchSettings(SettingsPatch{Theme: &unknown})
	if err != nil {
		t.Fatalf("PatchSettings(invalid theme) failed: %v", err)
	}
	if saved.Theme != "system" {
		t.Fatalf("Theme = %q, want system", saved.Theme)
	}

	// 缺少 autoCheckUpdate 字段的旧/手写 JSON 应保留默认 true，而非被零值覆盖为 false。
	rawPath := storage.SettingsPath(rootDir)
	if err := os.WriteFile(rawPath, []byte(`{"theme":"light"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write raw settings: %v", err)
	}
	got, err := store.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings on legacy partial JSON failed: %v", err)
	}
	if got.Theme != "light" || !got.AutoCheckUpdate {
		t.Fatalf("GetSettings() = %+v, want Theme=light AutoCheckUpdate=true", got)
	}

	// 损坏 JSON 应返回明确错误。
	if err := os.WriteFile(rawPath, []byte("{bad json"), 0o600); err != nil {
		t.Fatalf("write corrupted settings: %v", err)
	}
	if _, err := store.GetSettings(); err == nil {
		t.Fatal("expected error for corrupted settings JSON")
	}
}

func TestStoreSettingsConcurrentPatchAndPrepareStorage(t *testing.T) {
	rootDir := t.TempDir()
	store := NewStore(rootDir)

	modes := []string{"system", "light", "dark"}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mode := modes[idx%len(modes)]
			auto := idx%2 == 0
			if _, err := store.PatchSettings(SettingsPatch{
				Theme:           &mode,
				AutoCheckUpdate: &auto,
			}); err != nil {
				t.Errorf("concurrent PatchSettings(%d) failed: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	if _, err := os.Stat(filepath.Join(rootDir, "settings.json")); err != nil {
		t.Fatalf("expected settings.json to exist: %v", err)
	}
	store.PrepareStorage()
	if _, err := store.GetSettings(); err != nil {
		t.Fatalf("GetSettings after concurrent updates failed: %v", err)
	}
}
