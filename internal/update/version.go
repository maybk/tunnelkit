package update

import (
	"net/url"
	"strconv"
	"strings"
)

// IsValidReleaseURL 严格校验跳转链接是否指向本项目官方 GitHub Release 标签页。
func IsValidReleaseURL(rawURL, tag string) bool {
	if tag == "" || strings.ContainsAny(tag, "/?#\\") || tag == "." || tag == ".." {
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
