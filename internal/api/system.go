package api

import (
	"context"
	"runtime"

	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/update"
)

// SystemInfo 包含应用版本、运行平台、数据目录与内置 tunnelkit-helper 组件就绪状态。
type SystemInfo struct {
	AppVersion  string `json:"appVersion"`
	Platform    string `json:"platform"`
	HelperReady bool   `json:"helperReady"`
	HelperPath  string `json:"helperPath,omitempty"`
	DataDir     string `json:"dataDir,omitempty"`
}

// UpdateInfo 表示版本更新检查结果。
type UpdateInfo = update.Info

// UpdateProgress 表示应用内自更新的实时进度。
type UpdateProgress = update.Progress

// SystemAPI 暴露只读系统、组件就绪与版本更新检查及自更新接口。
type SystemAPI struct {
	version string
	dataDir string
	updater *update.Checker
}

func NewSystemAPI(version string, opts ...update.Option) *SystemAPI {
	return NewSystemAPIWithUpdater(version, "", update.NewChecker(version, opts...))
}

func NewSystemAPIWithUpdater(version, dataDir string, updater *update.Checker) *SystemAPI {
	if updater == nil {
		updater = update.NewChecker(version)
	}
	return &SystemAPI{
		version: version,
		dataDir: dataDir,
		updater: updater,
	}
}

func (a *SystemAPI) Info(_ context.Context) SystemInfo {
	helperPath, ready := helperclient.LocateHelper()
	return SystemInfo{
		AppVersion:  a.version,
		Platform:    runtime.GOOS,
		HelperReady: ready,
		HelperPath:  helperPath,
		DataDir:     a.dataDir,
	}
}

// CheckUpdate 检查 GitHub Releases 最新正式版本（优先使用加速源，失败时自动回退官方直连源）。
func (a *SystemAPI) CheckUpdate(ctx context.Context) (UpdateInfo, error) {
	return a.updater.Check(ctx)
}

// DownloadUpdate 下载并校验最新正式版更新安装包，支持通过 CancelUpdate 取消；完成后保留待安装。
func (a *SystemAPI) DownloadUpdate(ctx context.Context) error {
	_, err := a.updater.Download(ctx, nil)
	return err
}

// CancelUpdate 取消进行中的更新安装包下载并清理临时文件；未处于下载阶段时返回 false。
func (a *SystemAPI) CancelUpdate() bool {
	return a.updater.CancelDownload()
}

// InstallUpdate 对已下载的更新包二次校验 SHA-256、断开活动 VPN、原地替换组件并自动重启应用。
func (a *SystemAPI) InstallUpdate(ctx context.Context) error {
	return a.updater.InstallAndRestart(ctx)
}
