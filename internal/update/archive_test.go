package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractArchiveAndAtomicReplaceHelpers(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. 测试 ZIP 解压与缺失目标文件报错
	zipPath := filepath.Join(tmpDir, "pkg.zip")
	zipBuf := new(bytes.Buffer)
	zw := zip.NewWriter(zipBuf)
	for name, body := range map[string]string{
		"nested/tunnelkit.exe":        "new-gui-binary",
		"nested/tunnelkit-helper.exe": "new-helper-binary",
	} {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zw.Create: %v", err)
		}
		_, _ = fw.Write([]byte(body))
	}
	_ = zw.Close()
	if err := os.WriteFile(zipPath, zipBuf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	zipOutDir := filepath.Join(tmpDir, "zip-out")
	if err := os.MkdirAll(zipOutDir, 0o700); err != nil {
		t.Fatalf("mkdir zip-out: %v", err)
	}
	if err := ExtractZipTargets(zipPath, zipOutDir, map[string]os.FileMode{
		"tunnelkit.exe":        0o755,
		"tunnelkit-helper.exe": 0o755,
	}); err != nil {
		t.Fatalf("ExtractZipTargets failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(zipOutDir, "tunnelkit.exe")); err != nil {
		t.Fatalf("expected extracted tunnelkit.exe: %v", err)
	}
	if err := ExtractZipTargets(zipPath, zipOutDir, map[string]os.FileMode{
		"missing.exe": 0o755,
	}); err == nil {
		t.Fatal("expected error when required target is missing in zip")
	}

	// 2. 测试 TAR.GZ 解压与缺失目标文件报错
	tarGzPath := filepath.Join(tmpDir, "pkg.tar.gz")
	tarBuf := new(bytes.Buffer)
	gw := gzip.NewWriter(tarBuf)
	tw := tar.NewWriter(gw)
	for name, body := range map[string]string{
		"tunnelkit":        "linux-gui-binary",
		"tunnelkit-helper": "linux-helper-binary",
	} {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tw.WriteHeader: %v", err)
		}
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gw.Close()
	if err := os.WriteFile(tarGzPath, tarBuf.Bytes(), 0o600); err != nil {
		t.Fatalf("write tar.gz: %v", err)
	}

	tarOutDir := filepath.Join(tmpDir, "tar-out")
	if err := os.MkdirAll(tarOutDir, 0o700); err != nil {
		t.Fatalf("mkdir tar-out: %v", err)
	}
	if err := ExtractTarGzTargets(tarGzPath, tarOutDir, map[string]os.FileMode{
		"tunnelkit":        0o755,
		"tunnelkit-helper": 0o755,
	}); err != nil {
		t.Fatalf("ExtractTarGzTargets failed: %v", err)
	}
	if err := ExtractTarGzTargets(tarGzPath, tarOutDir, map[string]os.FileMode{
		"missing-bin": 0o755,
	}); err == nil {
		t.Fatal("expected error when required target is missing in tar.gz")
	}

	// 3. 测试备份替换（Windows 模式 useAtomicRename=false 与 Linux 模式 useAtomicRename=true）及回滚
	installDir := filepath.Join(tmpDir, "install")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("mkdir installDir: %v", err)
	}
	dstGUI := filepath.Join(installDir, "tunnelkit")
	dstHelper := filepath.Join(installDir, "tunnelkit-helper")
	if err := os.WriteFile(dstGUI, []byte("old-gui"), 0o755); err != nil {
		t.Fatalf("write old gui: %v", err)
	}
	if err := os.WriteFile(dstHelper, []byte("old-helper"), 0o755); err != nil {
		t.Fatalf("write old helper: %v", err)
	}

	if err := ReplaceFilesWithBackup(tarOutDir, installDir, []string{"tunnelkit", "tunnelkit-helper"}, 0o755, false); err != nil {
		t.Fatalf("ReplaceFilesWithBackup (copy mode) failed: %v", err)
	}
	gotGUI, err := os.ReadFile(dstGUI)
	if err != nil || string(gotGUI) != "linux-gui-binary" {
		t.Fatalf("unexpected dstGUI content: %q (err=%v)", gotGUI, err)
	}
	if _, err := os.Stat(dstGUI + ".old"); err != nil {
		t.Fatalf("expected backup file %s.old to exist: %v", dstGUI, err)
	}

	CleanDirectoryBackups(installDir)
	if _, err := os.Stat(dstGUI + ".old"); !os.IsNotExist(err) {
		t.Fatalf("expected CleanDirectoryBackups to remove .old file, err=%v", err)
	}

	if err := ReplaceFilesWithBackup(tarOutDir, installDir, []string{"tunnelkit", "tunnelkit-helper"}, 0o755, true); err != nil {
		t.Fatalf("ReplaceFilesWithBackup (atomic rename mode) failed: %v", err)
	}

	// 测试当第二个源文件不存在时自动回滚第一个文件
	if err := os.WriteFile(dstGUI, []byte("rollback-baseline"), 0o755); err != nil {
		t.Fatalf("write baseline gui: %v", err)
	}
	if err := ReplaceFilesWithBackup(tarOutDir, installDir, []string{"tunnelkit", "non-existent-file"}, 0o755, false); err == nil {
		t.Fatal("expected error when replacing non-existent file")
	}
	rolledBack, err := os.ReadFile(dstGUI)
	if err != nil || string(rolledBack) != "rollback-baseline" {
		t.Fatalf("expected dstGUI to rollback to 'rollback-baseline', got %q (err=%v)", rolledBack, err)
	}

	// 4. 测试路径解析与清理辅助函数
	CleanupOldArtifacts()
	if exe, err := ResolveExecutablePath(); err != nil || exe == "" {
		t.Fatalf("ResolveExecutablePath failed: exe=%q err=%v", exe, err)
	}
	bundle, ok := ResolveMacAppBundlePath("/Applications/tunnelkit.app/Contents/MacOS/tunnelkit")
	if !ok || filepath.ToSlash(bundle) != "/Applications/tunnelkit.app" {
		t.Fatalf("unexpected bundle path: %q (ok=%v)", bundle, ok)
	}
	if _, ok := ResolveMacAppBundlePath("/usr/local/bin/tunnelkit"); ok {
		t.Fatal("expected ok=false for non-app-bundle path")
	}
}
