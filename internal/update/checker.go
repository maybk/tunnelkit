// Package update 提供应用版本更新检查、多源降级回退、发布页地址校验、应用内自更新与语义化版本比较。
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	releaseRepoOwner           = "maybk"
	releaseRepoName            = "tunnelkit"
	defaultProxyPrefix         = "https://gh-proxy.com/"
	defaultReleaseDirectAPIURL = "https://api.github.com/repos/" + releaseRepoOwner + "/" + releaseRepoName + "/releases/latest"
	defaultReleaseProxyAPIURL  = defaultProxyPrefix + defaultReleaseDirectAPIURL
	perSourceCheckTimeout      = 6 * time.Second
	maxReleaseResponseBytes    = 1 << 20 // 1 MiB
	sha256SumsAssetName        = "SHA256SUMS.txt"
)

var (
	ErrEmptyCurrentVersion = errors.New("无法读取当前应用版本")
	ErrNoPublishedRelease  = errors.New("更新源暂无已发布版本")
	ErrInvalidReleaseInfo  = errors.New("版本信息格式无效")
	ErrCheckUpdateFailed   = errors.New("检查版本更新失败，请稍后重试")
	ErrUpdateInProgress    = errors.New("正在执行版本更新，请勿重复操作")
	ErrSelfUpdateNotReady  = errors.New("当前平台或版本暂不支持应用内自动更新，请前往下载页手动更新")
	ErrDownloadUpdate      = errors.New("下载更新安装包失败，请稍后重试或前往下载页手动下载")
	ErrChecksumMismatch    = errors.New("更新安装包 SHA-256 校验失败，已中止安装")
)

// Info 表示版本更新检查结果。
type Info struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	AssetName      string `json:"assetName,omitempty"`
	AssetURL       string `json:"assetUrl,omitempty"`
	AssetSize      int64  `json:"assetSize,omitempty"`
	AssetSHA256    string `json:"assetSha256,omitempty"`
	CanSelfUpdate  bool   `json:"canSelfUpdate"`
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

// WithPlatform 自定义更新检查与资产匹配所使用的目标操作系统和架构（主要用于测试）。
func WithPlatform(goos, goarch string) Option {
	return func(c *Checker) {
		if goos != "" {
			c.goos = goos
		}
		if goarch != "" {
			c.goarch = goarch
		}
	}
}

// WithProxyPrefix 自定义下载加速源前缀（传空字符串可禁用加速前缀）。
func WithProxyPrefix(prefix string) Option {
	return func(c *Checker) {
		c.proxyPrefix = prefix
	}
}

// WithApplier 自定义安装包落盘后的平台安装器（主要用于测试）。
func WithApplier(applier ApplierFunc) Option {
	return func(c *Checker) {
		if applier != nil {
			c.applier = applier
		}
	}
}

// WithAssetURLValidator 自定义资产下载 URL 校验器（仅用于本地 httptest 测试覆盖）。
func WithAssetURLValidator(validator func(rawURL, tag, assetName string) bool) Option {
	return func(c *Checker) {
		if validator != nil {
			c.assetURLValidator = validator
		}
	}
}

// WithPreUpdateHook 注册执行应用内更新前的准备钩子（如自动断开当前 VPN 连接）。
func WithPreUpdateHook(fn func(context.Context) error) Option {
	return func(c *Checker) {
		if fn != nil {
			c.preUpdateHook = fn
		}
	}
}

// WithProgressListener 注册自更新进度广播监听器（如向前端发送事件）。
func WithProgressListener(fn ProgressFunc) Option {
	return func(c *Checker) {
		if fn != nil {
			c.progressListener = fn
		}
	}
}

// WithRestartHandler 自定义自更新完成后的进程重启处理器。
func WithRestartHandler(fn func(restartFn func() error) error) Option {
	return func(c *Checker) {
		if fn != nil {
			c.restartHandler = fn
		}
	}
}

// Checker 负责向 GitHub Releases 查询最新正式版本、判断是否存在更新并驱动应用内自更新。
type Checker struct {
	version           string
	goos              string
	goarch            string
	proxyPrefix       string
	httpClient        *http.Client
	releaseURLs       []string
	applier           ApplierFunc
	assetURLValidator func(rawURL, tag, assetName string) bool
	preUpdateHook     func(context.Context) error
	progressListener  ProgressFunc
	restartHandler    func(restartFn func() error) error

	mu         sync.Mutex
	updating   bool
	cachedInfo *Info
}

// NewChecker 创建版本更新检查器，默认优先请求加速源，失败时自动回退至 GitHub 官方源。
func NewChecker(version string, opts ...Option) *Checker {
	c := &Checker{
		version:     version,
		goos:        runtime.GOOS,
		goarch:      runtime.GOARCH,
		proxyPrefix: defaultProxyPrefix,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		releaseURLs: []string{
			defaultReleaseProxyAPIURL,
			defaultReleaseDirectAPIURL,
		},
		applier:           applyPlatformUpdate,
		assetURLValidator: IsValidAssetDownloadURL,
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
			c.mu.Lock()
			copied := info
			c.cachedInfo = &copied
			c.mu.Unlock()
			return info, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return Info{}, lastErr
	}
	return Info{}, ErrCheckUpdateFailed
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

type githubReleasePayload struct {
	TagName    string               `json:"tag_name"`
	HTMLURL    string               `json:"html_url"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
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

	hasUpdate := IsNewerVersion(currentVersion, latest)
	info := Info{
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		HasUpdate:      hasUpdate,
		ReleaseURL:     releaseURL,
	}

	if matched, ok := c.resolvePlatformAsset(parentCtx, tag, latest, payload.Assets); ok {
		info.AssetName = matched.Name
		info.AssetURL = matched.BrowserDownloadURL
		info.AssetSize = matched.Size
		info.AssetSHA256 = matched.Digest
		info.CanSelfUpdate = hasUpdate && info.AssetSHA256 != ""
	}

	return info, nil
}

func (c *Checker) resolvePlatformAsset(ctx context.Context, tag, latest string, assets []githubReleaseAsset) (githubReleaseAsset, bool) {
	candidates := ExpectedAssetNames(latest, c.goos, c.goarch)
	if len(candidates) == 0 || len(assets) == 0 {
		return githubReleaseAsset{}, false
	}

	validateURL := c.assetURLValidator
	if validateURL == nil {
		validateURL = IsValidAssetDownloadURL
	}

	byName := make(map[string]githubReleaseAsset, len(assets))
	for _, a := range assets {
		name := strings.TrimSpace(a.Name)
		dlURL := strings.TrimSpace(a.BrowserDownloadURL)
		if name == "" || dlURL == "" {
			continue
		}
		a.Name = name
		a.BrowserDownloadURL = dlURL
		byName[name] = a
	}

	for _, candidate := range candidates {
		asset, exists := byName[candidate]
		if !exists || !validateURL(asset.BrowserDownloadURL, tag, asset.Name) {
			continue
		}
		if sum, ok := ParseSHA256Digest(asset.Digest); ok {
			asset.Digest = sum
			return asset, true
		}
		// 若 GitHub API 资产未直接携带 digest 字段，尝试从同 Release 的 SHA256SUMS.txt 获取。
		if sumsAsset, hasSums := byName[sha256SumsAssetName]; hasSums &&
			validateURL(sumsAsset.BrowserDownloadURL, tag, sha256SumsAssetName) {
			if sum, ok := c.fetchChecksumFromSumsAsset(ctx, sumsAsset.BrowserDownloadURL, asset.Name); ok {
				asset.Digest = sum
				return asset, true
			}
		}
	}
	return githubReleaseAsset{}, false
}

func (c *Checker) fetchChecksumFromSumsAsset(parentCtx context.Context, sumsURL, targetAsset string) (string, bool) {
	for _, u := range c.candidateDownloadURLs(sumsURL) {
		ctx, cancel := context.WithTimeout(parentCtx, perSourceCheckTimeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			continue
		}
		client := c.httpClient
		if client == nil {
			client = http.DefaultClient
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		cancel()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if sum, ok := ParseSHA256Sums(string(body), targetAsset); ok {
			return sum, true
		}
	}
	return "", false
}

func (c *Checker) candidateDownloadURLs(directURL string) []string {
	prefix := strings.TrimSpace(c.proxyPrefix)
	if prefix == "" {
		return []string{directURL}
	}
	if prefix == defaultProxyPrefix && !strings.HasPrefix(directURL, "https://github.com/") {
		return []string{directURL}
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return []string{prefix + directURL, directURL}
}
