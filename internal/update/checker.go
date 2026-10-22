// Package update 提供应用版本更新检查、多源降级回退、发布页地址校验与语义化版本比较。
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	releaseRepoOwner           = "maybk"
	releaseRepoName            = "tunnelkit"
	defaultReleaseDirectAPIURL = "https://api.github.com/repos/" + releaseRepoOwner + "/" + releaseRepoName + "/releases/latest"
	defaultReleaseProxyAPIURL  = "https://gh-proxy.com/" + defaultReleaseDirectAPIURL
	perSourceCheckTimeout      = 6 * time.Second
	maxReleaseResponseBytes    = 1 << 20 // 1 MiB
)

var (
	ErrEmptyCurrentVersion = errors.New("无法读取当前应用版本")
	ErrNoPublishedRelease  = errors.New("更新源暂无已发布版本")
	ErrInvalidReleaseInfo  = errors.New("版本信息格式无效")
	ErrCheckUpdateFailed   = errors.New("检查版本更新失败，请稍后重试")
)

// Info 表示版本更新检查结果。
type Info struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
}

// Option 定义 Checker 的可选配置项。
type Option func(*Checker)

// WithHTTPClient 自定义更新检查所使用的 HTTP 客户端。
func WithHTTPClient(client *http.Client) Option {
	return func(c *Checker) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithReleaseURLs 自定义更新检查的候选接口列表（按优先级顺序尝试）。
func WithReleaseURLs(urls ...string) Option {
	return func(c *Checker) {
		if len(urls) > 0 {
			c.releaseURLs = append([]string(nil), urls...)
		}
	}
}

// Checker 负责向 GitHub Releases 查询最新正式版本并判断是否存在更新。
type Checker struct {
	version     string
	httpClient  *http.Client
	releaseURLs []string
}

// NewChecker 创建版本更新检查器，默认优先请求加速源，失败时自动回退至 GitHub 官方源。
func NewChecker(version string, opts ...Option) *Checker {
	c := &Checker{
		version:    version,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		releaseURLs: []string{
			defaultReleaseProxyAPIURL,
			defaultReleaseDirectAPIURL,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Check 检查最新正式版本并与当前版本比较。
func (c *Checker) Check(ctx context.Context) (Info, error) {
	current := strings.TrimPrefix(strings.TrimSpace(c.version), "v")
	if current == "" {
		return Info{}, ErrEmptyCurrentVersion
	}

	urls := c.releaseURLs
	if len(urls) == 0 {
		urls = []string{defaultReleaseProxyAPIURL, defaultReleaseDirectAPIURL}
	}

	var lastErr error
	for _, endpoint := range urls {
		if err := ctx.Err(); err != nil {
			return Info{}, err
		}
		info, err := c.fetchReleaseFrom(ctx, endpoint, current)
		if err == nil {
			return info, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return Info{}, lastErr
	}
	return Info{}, ErrCheckUpdateFailed
}

type githubReleasePayload struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (c *Checker) fetchReleaseFrom(parentCtx context.Context, endpoint, currentVersion string) (Info, error) {
	ctx, cancel := context.WithTimeout(parentCtx, perSourceCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Info{}, ErrCheckUpdateFailed
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("TunnelKit/%s", currentVersion))

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if parentCtx.Err() != nil {
			return Info{}, parentCtx.Err()
		}
		return Info{}, ErrCheckUpdateFailed
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Info{}, ErrNoPublishedRelease
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, ErrCheckUpdateFailed
	}

	var payload githubReleasePayload
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseResponseBytes))
	if err := dec.Decode(&payload); err != nil {
		return Info{}, ErrInvalidReleaseInfo
	}

	tag := strings.TrimSpace(payload.TagName)
	latest := strings.TrimPrefix(tag, "v")
	releaseURL := strings.TrimSpace(payload.HTMLURL)
	if tag == "" || latest == "" || payload.Draft || payload.Prerelease || !IsValidReleaseURL(releaseURL, tag) {
		return Info{}, ErrInvalidReleaseInfo
	}
	if _, ok := parseVersionParts(latest); !ok {
		return Info{}, ErrInvalidReleaseInfo
	}

	return Info{
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		HasUpdate:      IsNewerVersion(currentVersion, latest),
		ReleaseURL:     releaseURL,
	}, nil
}
