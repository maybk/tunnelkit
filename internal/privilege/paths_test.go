package privilege

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComponentDigestTracksPackagedChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte("helper revision one"), 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := digest(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("helper revision two"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := digest(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("component upgrade must require installation")
	}
}
