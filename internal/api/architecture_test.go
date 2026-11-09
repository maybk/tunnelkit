package api

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestBackendArchitectureBoundaries(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))

	t.Run("fortivpn stays decoupled from internal packages and Wails", func(t *testing.T) {
		fortivpnDir := filepath.Join(repoRoot, "fortivpn")
		err := filepath.WalkDir(fortivpnDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			for _, imp := range parseFileImports(t, path) {
				if strings.HasPrefix(imp, "tunnelkit/internal") || strings.Contains(imp, "wailsapp/wails") {
					rel, _ := filepath.Rel(repoRoot, path)
					t.Errorf("%s imports forbidden package %q", rel, imp)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk fortivpn: %v", err)
		}
	})

	t.Run("desktop GUI packages do not import fortivpn/engine", func(t *testing.T) {
		mainImports := parseFileImports(t, filepath.Join(repoRoot, "main.go"))
		for _, imp := range mainImports {
			if imp == "tunnelkit/fortivpn/engine" || strings.HasPrefix(imp, "tunnelkit/fortivpn/engine/") {
				t.Errorf("main.go imports forbidden package %q", imp)
			}
		}

		internalDir := filepath.Join(repoRoot, "internal")
		helperDir := filepath.Join(internalDir, "helper")
		err := filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == helperDir {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			for _, imp := range parseFileImports(t, path) {
				if imp == "tunnelkit/fortivpn/engine" || strings.HasPrefix(imp, "tunnelkit/fortivpn/engine/") {
					rel, _ := filepath.Rel(repoRoot, path)
					t.Errorf("%s imports forbidden package %q", rel, imp)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk internal: %v", err)
		}
	})

	t.Run("internal/api remains a thin adapter without direct file I/O or lower-layer imports", func(t *testing.T) {
		entries, err := filepath.Glob("*.go")
		if err != nil {
			t.Fatalf("glob internal/api: %v", err)
		}
		allowed := map[string]bool{
			"context":                         true,
			"errors":                          true,
			"fmt":                             true,
			"log/slog":                        true,
			"runtime":                         true,
			"strings":                         true,
			"tunnelkit/fortivpn":              true,
			"tunnelkit/internal/helperclient": true,
			"tunnelkit/internal/profile":      true,
			"tunnelkit/internal/update":       true,
			"tunnelkit/internal/vpn":          true,
		}
		for _, file := range entries {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			raw, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatalf("read internal/api/%s: %v", file, readErr)
			}
			lineCount := strings.Count(string(raw), "\n") + 1
			if lineCount > 160 {
				t.Errorf("internal/api/%s has %d lines, want <= 160", file, lineCount)
			}
			for _, imp := range parseFileImports(t, file) {
				if !allowed[imp] {
					t.Errorf("internal/api/%s imports forbidden package %q", file, imp)
				}
			}
		}
	})

	t.Run("privileged helper stays decoupled from desktop GUI and Wails", func(t *testing.T) {
		helperDirs := []string{
			filepath.Join(repoRoot, "cmd", "tunnelkit-helper"),
			filepath.Join(repoRoot, "internal", "helper"),
		}
		for _, dir := range helperDirs {
			err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
					return nil
				}
				for _, imp := range parseFileImports(t, path) {
					if strings.Contains(imp, "wailsapp/wails") ||
						(strings.HasPrefix(imp, "tunnelkit/internal/") &&
							imp != "tunnelkit/internal/helper" &&
							imp != "tunnelkit/internal/ipc" &&
							imp != "tunnelkit/internal/privilege") {
						rel, _ := filepath.Rel(repoRoot, path)
						t.Errorf("%s imports forbidden package %q", rel, imp)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s: %v", dir, err)
			}
		}
	})
}

func parseFileImports(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports of %s: %v", path, err)
	}
	out := make([]string, 0, len(f.Imports))
	for _, spec := range f.Imports {
		val, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		out = append(out, val)
	}
	return out
}
