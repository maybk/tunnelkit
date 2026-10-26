package api

import (
	"context"
	"runtime"

	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/update"
)

// SystemInfo 包含应用版本、运行平台与内置 tunnelkit-helper 组件就绪状态。
type SystemInfo struct {
	AppVersion  string `json:"appVersion"`
	Platform    string `json:"platform"`
	HelperReady bool   `json:"helperReady"`
	HelperPath  string `json:"helperPath,omitempty"`
}

// UpdateInfo 表示版本更新检查结果。
type UpdateInfo = update.Info

// UpdateProgress 表示应用内自更新的实时进度。
type UpdateProgress = update.Progress

// SystemAPI 暴露只读系统、组件就绪与版本更新检查及自更新接口。
type SystemAPI struct {
	version string
	updater *update.Checker
}

func NewSystemAPI(version string, opts ...update.Option) *SystemAPI {
	return &SystemAPI{
		version: version,
		updater: update.NewChecker(version, opts...),
	}
}

func (a *SystemAPI) Info(_ context.Context) SystemInfo {
	helperPath, ready := helperclient.LocateHelper()
	return SystemInfo{
		AppVersion:  a.version,
		Platform:    runtime.GOOS,
		HelperReady: ready,
		HelperPath:  helperPath,
	}
}

// CheckUpdate 检查 GitHub Releases 最新正式版本（优先使用加速源，失败时自动回退官方直连源）。
func (a *SystemAPI) CheckUpdate(ctx context.Context) (UpdateInfo, error) {
	return a.updater.Check(ctx)
}

// ApplyUpdate 下载、校验 SHA-256、原地替换最新正式版安装包并自动重启应用。
func (a *SystemAPI) ApplyUpdate(ctx context.Context) error {
	return a.updater.ApplyAndRestart(ctx)
}
