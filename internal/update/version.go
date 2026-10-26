package update

import (
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
)

// IsValidReleaseURL 严格校验跳转链接是否指向本项目官方 GitHub Release 标签页。
func IsValidReleaseURL(rawURL, tag string) bool {
	if !isSafePathToken(tag) {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") {
		return false
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(segments) != 5 {
		return false
	}
	return strings.EqualFold(segments[0], releaseRepoOwner) &&
		strings.EqualFold(segments[1], releaseRepoName) &&
		segments[2] == "releases" &&
		segments[3] == "tag" &&
		segments[4] == tag
}

// IsValidAssetDownloadURL 严格校验安装包下载链接是否指向本项目官方 GitHub Release 指定标签与文件名。
func IsValidAssetDownloadURL(rawURL, tag, assetName string) bool {
	if !isSafePathToken(tag) || !isSafePathToken(assetName) {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") {
		return false
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(segments) != 6 {
		return false
	}
	return strings.EqualFold(segments[0], releaseRepoOwner) &&
		strings.EqualFold(segments[1], releaseRepoName) &&
		segments[2] == "releases" &&
		segments[3] == "download" &&
		segments[4] == tag &&
		segments[5] == assetName
}

func isSafePathToken(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/?#\\") && s != "." && s != ".."
}

// ExpectedAssetNames 返回指定平台与架构用于应用内自更新的候选发布资产文件名列表（按优先级排序）。
func ExpectedAssetNames(version, goos, goarch string) []string {
	ver := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if ver == "" {
		return nil
	}
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return []string{
				"tunnelkit-" + ver + "-macos-arm64.dmg",
				"tunnelkit-" + ver + "-macos-universal.dmg",
			}
		case "amd64":
			return []string{
				"tunnelkit-" + ver + "-macos-amd64.dmg",
				"tunnelkit-" + ver + "-macos-universal.dmg",
			}
		default:
			return []string{
				"tunnelkit-" + ver + "-macos-universal.dmg",
			}
		}
	case "windows":
		if goarch == "amd64" || goarch == "arm64" {
			return []string{
				"tunnelkit-" + ver + "-windows-" + goarch + ".zip",
			}
		}
	case "linux":
		if goarch == "amd64" || goarch == "arm64" {
			return []string{
				"tunnelkit-" + ver + "-linux-" + goarch + ".tar.gz",
			}
		}
	}
	return nil
}

// ParseSHA256Digest 解析并校验形如 "sha256:<64位hex>" 或 "<64位hex>" 的摘要字符串。
func ParseSHA256Digest(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(s), "sha256:") {
		s = strings.TrimSpace(s[len("sha256:"):])
	}
	if len(s) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", false
	}
	return strings.ToLower(s), true
}

// ParseSHA256Sums 从 SHA256SUMS.txt 内容中查找指定文件名的 SHA-256 哈希。
func ParseSHA256Sums(content, targetAsset string) (string, bool) {
	if targetAsset == "" {
		return "", false
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if name == targetAsset {
			if sum, ok := ParseSHA256Digest(fields[0]); ok {
				return sum, true
			}
		}
	}
	return "", false
}

// IsNewerVersion 判断 latest 语义化版本号是否比 current 更新。
func IsNewerVersion(current, latest string) bool {
	curParts, curOK := parseVersionParts(current)
	latParts, latOK := parseVersionParts(latest)
	if !curOK || !latOK {
		return false
	}
	maxLen := len(curParts)
	if len(latParts) > maxLen {
		maxLen = len(latParts)
	}
	for i := 0; i < maxLen; i++ {
		var curVal, latVal int
		if i < len(curParts) {
			curVal = curParts[i]
		}
		if i < len(latParts) {
			latVal = latParts[i]
		}
		if latVal != curVal {
			return latVal > curVal
		}
	}
	// 主版本号相同时，仅当当前版本为预发布版（带 -）且目标版本为正式版时视为有更新。
	curPre := hasPrereleaseSuffix(current)
	latPre := hasPrereleaseSuffix(latest)
	return curPre && !latPre
}

func hasPrereleaseSuffix(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus]
	}
	return strings.Contains(v, "-")
}

func parseVersionParts(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil, false
	}
	if cut := strings.IndexAny(v, "-+"); cut >= 0 {
		v = v[:cut]
	}
	rawParts := strings.Split(v, ".")
	if len(rawParts) == 0 {
		return nil, false
	}
	parts := make([]int, 0, len(rawParts))
	for _, p := range rawParts {
		if p == "" {
			return nil, false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}
