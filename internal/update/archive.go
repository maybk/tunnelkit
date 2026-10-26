package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxExtractedBinaryBytes = 250 << 20 // 250 MiB

// ResolveExecutablePath 获取当前进程可执行文件的真实绝对路径（自动解析符号链接）。
func ResolveExecutablePath() (string, error) {
	selfExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位当前可执行文件失败: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(selfExe); err == nil {
		selfExe = resolved
	}
	return selfExe, nil
}

// CleanupOldArtifacts 清理上次自更新在当前可执行文件同目录下遗留的 .old 备份文件。
func CleanupOldArtifacts() {
	selfExe, err := ResolveExecutablePath()
	if err != nil {
		return
	}
	CleanDirectoryBackups(filepath.Dir(selfExe))
}

// CleanDirectoryBackups 清理指定目录中的自更新备份文件。
func CleanDirectoryBackups(dir string) {
	for _, name := range []string{
		"tunnelkit.exe.old",
		"tunnelkit-helper.exe.old",
		"wintun.dll.old",
		"tunnelkit.old",
		"tunnelkit-helper.old",
	} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// ExtractZipTargets 从 ZIP 归档中按文件名提取指定文件集到 destDir。
func ExtractZipTargets(zipPath, destDir string, requiredFiles map[string]os.FileMode) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开更新压缩包失败: %w", err)
	}
	defer r.Close()

	extracted := make(map[string]bool, len(requiredFiles))
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(filepath.Clean(filepath.FromSlash(f.Name)))
		mode, ok := requiredFiles[base]
		if !ok {
			continue
		}
		if err := extractSingleStream(destDir, base, mode, f.Open); err != nil {
			return err
		}
		extracted[base] = true
	}

	for name := range requiredFiles {
		if !extracted[name] {
			return fmt.Errorf("更新安装包缺少必要组件: %s", name)
		}
	}
	return nil
}

// ExtractTarGzTargets 从 .tar.gz 归档中按文件名提取指定文件集到 destDir。
func ExtractTarGzTargets(tarGzPath, destDir string, requiredFiles map[string]os.FileMode) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("打开更新归档失败: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("解析更新归档失败: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	extracted := make(map[string]bool, len(requiredFiles))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("读取更新归档条目失败: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		base := filepath.Base(filepath.Clean(filepath.FromSlash(hdr.Name)))
		mode, ok := requiredFiles[base]
		if !ok {
			continue
		}
		if err := writeLimitedFile(filepath.Join(destDir, base), tr, mode); err != nil {
			return err
		}
		extracted[base] = true
	}

	for name := range requiredFiles {
		if !extracted[name] {
			return fmt.Errorf("更新归档缺少必要组件: %s", name)
		}
	}
	return nil
}

func extractSingleStream(destDir, baseName string, mode os.FileMode, openFn func() (io.ReadCloser, error)) error {
	rc, err := openFn()
	if err != nil {
		return err
	}
	defer rc.Close()
	return writeLimitedFile(filepath.Join(destDir, baseName), rc, mode)
}

func writeLimitedFile(targetPath string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	limited := io.LimitReader(r, maxExtractedBinaryBytes+1)
	n, copyErr := io.Copy(out, limited)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(targetPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(targetPath)
		return closeErr
	}
	if n > maxExtractedBinaryBytes {
		_ = os.Remove(targetPath)
		return errors.New("更新组件体积超出安全上限")
	}
	return os.Chmod(targetPath, mode)
}

// ReplaceFilesWithBackup 将 stageDir 中的文件原子替换到 targetDir（原文件先重命名为 .old，若中途失败自动回滚）。
func ReplaceFilesWithBackup(stageDir, targetDir string, names []string, mode os.FileMode, removeBackup bool) error {
	var backedUp []string
	var replaced []string

	rollback := func() {
		for _, name := range replaced {
			_ = os.Remove(filepath.Join(targetDir, name))
		}
		for _, name := range backedUp {
			oldPath := filepath.Join(targetDir, name+".old")
			dstPath := filepath.Join(targetDir, name)
			_ = os.Rename(oldPath, dstPath)
		}
	}

	for _, name := range names {
		srcPath := filepath.Join(stageDir, name)
		dstPath := filepath.Join(targetDir, name)
		oldPath := dstPath + ".old"

		_ = os.Remove(oldPath)
		if _, err := os.Stat(dstPath); err == nil {
			if err := os.Rename(dstPath, oldPath); err != nil {
				rollback()
				return err
			}
			backedUp = append(backedUp, name)
		}

		if err := copyAndRenameFile(srcPath, dstPath, mode); err != nil {
			rollback()
			return err
		}
		replaced = append(replaced, name)
	}

	if removeBackup {
		for _, name := range backedUp {
			_ = os.Remove(filepath.Join(targetDir, name+".old"))
		}
	}
	return nil
}

func copyAndRenameFile(srcPath, dstPath string, mode os.FileMode) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	tmpPath := dstPath + ".new"
	_ = os.Remove(tmpPath)
	if err := writeLimitedFile(tmpPath, in, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// ResolveMacAppBundlePath 从可执行文件路径向上查找所属的 .app 包根目录。
func ResolveMacAppBundlePath(exePath string) (string, bool) {
	cleaned := filepath.Clean(exePath)
	dir := filepath.Dir(cleaned)
	if filepath.Base(dir) != "MacOS" {
		return "", false
	}
	contentsDir := filepath.Dir(dir)
	if filepath.Base(contentsDir) != "Contents" {
		return "", false
	}
	bundleDir := filepath.Dir(contentsDir)
	if !strings.HasSuffix(strings.ToLower(bundleDir), ".app") {
		return "", false
	}
	return bundleDir, true
}
