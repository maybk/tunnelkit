//go:build darwin || linux

package privilege

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

func componentPairs(source, target string) [][2]string { return [][2]string{{source, target}} }

const maxTrustedSymlinkHops = 40

// ResolveTrustedSystemBinary 从候选绝对路径中校验并返回由 root 拥有且不可被群组或其他用户写入的系统可执行文件。
// 逐级校验所有祖先目录与系统符号链接跳点（兼容 Linux usrmerge 如 /bin -> usr/bin、Debian alternatives 与 sudo-rs），
// 并返回经校验的入口路径以保留多合一二进制 argv[0] 语义及系统安全策略路径匹配。
func ResolveTrustedSystemBinary(candidates ...string) (string, bool) {
	return resolveTrustedBinaryAt("/", 0, candidates...)
}

func resolveTrustedBinaryAt(rootDir string, trustedUID uint32, candidates ...string) (string, bool) {
	rootDir = filepath.Clean(rootDir)
	if err := checkProtectedUID(rootDir, true, trustedUID); err != nil {
		return "", false
	}
	for _, p := range candidates {
		if !filepath.IsAbs(p) || p == "/" || p != filepath.Clean(p) {
			continue
		}
		if resolveSingleTrustedBinary(rootDir, trustedUID, p) {
			if rootDir == "/" {
				return p, true
			}
			return filepath.Join(rootDir, strings.TrimPrefix(p, "/")), true
		}
	}
	return "", false
}

func splitPathSegments(rel string) []string {
	raw := strings.Split(rel, "/")
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s == "" || s == "." {
			continue
		}
		out = append(out, s)
	}
	return out
}

func resolveSingleTrustedBinary(rootDir string, trustedUID uint32, cleanCandidate string) bool {
	parts := splitPathSegments(strings.TrimPrefix(cleanCandidate, "/"))
	if len(parts) == 0 {
		return false
	}
	curRel := ""
	hops := 0
	for len(parts) > 0 {
		seg := parts[0]
		parts = parts[1:]
		if seg == ".." {
			if curRel != "" {
				curRel = filepath.Dir(curRel)
				if curRel == "." {
					curRel = ""
				}
			}
			if err := checkProtectedUID(filepath.Join(rootDir, curRel), true, trustedUID); err != nil {
				return false
			}
			continue
		}

		nextRel := seg
		if curRel != "" {
			nextRel = curRel + "/" + seg
		}
		nextFull := filepath.Join(rootDir, nextRel)
		info, err := os.Lstat(nextFull)
		if err != nil {
			return false
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != trustedUID {
			return false
		}

		if info.Mode()&os.ModeSymlink != 0 {
			hops++
			if hops > maxTrustedSymlinkHops {
				return false
			}
			target, err := os.Readlink(nextFull)
			if err != nil || target == "" {
				return false
			}
			if len(parts) == 0 && strings.HasSuffix(target, "/") {
				return false
			}
			linkParts := splitPathSegments(strings.TrimPrefix(target, "/"))
			if strings.HasPrefix(target, "/") {
				curRel = ""
			}
			if len(linkParts) == 0 && len(parts) == 0 {
				return false
			}
			parts = append(linkParts, parts...)
			continue
		}

		if len(parts) > 0 {
			if !info.IsDir() || info.Mode()&0o022 != 0 {
				return false
			}
			curRel = nextRel
			continue
		}

		return info.Mode().IsRegular() && info.Mode()&0o111 != 0 && info.Mode()&0o022 == 0
	}
	return false
}

func checkProtected(path string, directory bool) error {
	return checkProtectedUID(path, directory, 0)
}

func checkProtectedUID(path string, directory bool, uid uint32) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uid || info.Mode()&0o022 != 0 || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory {
		return fmt.Errorf("unsafe helper path permissions: %s", path)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("invalid helper component: %s", path)
	}
	return nil
}

func protectDirectory(path string) error {
	// Resolve system aliases such as macOS /var once; every descendant must be
	// root-owned and non-writable to ordinary users before proceeding.
	parent := filepath.Dir(path)
	if parent != path {
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			if err := protectDirectory(parent); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return err
		}
		if err := verifyParents(resolved); err != nil {
			return err
		}
	}
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := checkProtected(path, true); err != nil {
		return err
	}
	// Directory traversal must not depend on the installing user's umask.
	return os.Chmod(path, 0o755)
}

func verifyParents(path string) error {
	for {
		if err := checkProtected(path, true); err != nil {
			return err
		}
		next := filepath.Dir(path)
		if next == path {
			return nil
		}
		path = next
	}
}

func protectComponent(path string, mode os.FileMode) error {
	if err := os.Chown(path, 0, 0); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func verifyExistingAncestors(dir string) (bool, error) {
	cur := dir
	missing := false
	for {
		info, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			missing = true
			next := filepath.Dir(cur)
			if next == cur {
				return true, fmt.Errorf("missing root directory for %s", dir)
			}
			cur = next
			continue
		}
		if err != nil {
			return false, err
		}
		if !missing {
			if err := checkProtected(cur, true); err != nil {
				return false, err
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				return false, nil
			}
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return false, err
			}
			return false, verifyParents(resolved)
		}
		if info.Mode()&os.ModeSymlink != 0 && cur == dir {
			return false, fmt.Errorf("unsafe helper path permissions: %s", cur)
		}
		resolved, err := filepath.EvalSymlinks(cur)
		if err != nil {
			return false, err
		}
		return true, verifyParents(resolved)
	}
}

func inspectInstalled(path string) (string, error) {
	dirMissing, err := verifyExistingAncestors(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	if dirMissing {
		return "系统辅助目录未安装", nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "系统辅助程序未安装", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("invalid helper component: %s", path)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("unsupported file stat: %s", path)
	}
	if st.Uid != 0 || info.Mode()&0o022 != 0 || info.Mode()&os.ModeSetuid == 0 || info.Mode()&0o111 == 0 {
		return "系统辅助程序权限未就绪", nil
	}
	return "", nil
}

// Install 将辅助程序稳定部署至受保护的系统目录并设置 root setuid 权限。
func Install(source string) error {
	if callerUID != 0 || os.Geteuid() != 0 {
		return errors.New("helper installation requires fresh administrator authorization")
	}
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	if err := protectDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	base, err := stateRoot()
	if err != nil {
		return err
	}
	if err := protectDirectory(base); err != nil {
		return err
	}
	lock, err := AcquireSession()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := copyComponent(source, target, os.ModeSetuid|0o755); err != nil {
		return err
	}
	return verifyInstalled(target)
}

// Uninstall 仅移除已安装的辅助程序二进制，保留恢复状态以便后续重装后仍可恢复中断会话。
func Uninstall() error {
	if callerUID != 0 || os.Geteuid() != 0 {
		return errors.New("helper removal requires administrator authorization")
	}
	lock, err := AcquireSession()
	if err != nil {
		return err
	}
	defer lock.Close()
	target, err := ExecutablePath()
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := os.Remove("/usr/libexec/tunnelkit"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
