package api

import (
	"context"
	"runtime"

	"tunnelkit/internal/helperclient"
)

// SystemInfo 包含应用版本、运行平台与内置 tunnelkit-helper 组件就绪状态。
type SystemInfo struct {
	AppVersion  string `json:"appVersion"`
	Platform    string `json:"platform"`
	HelperReady bool   `json:"helperReady"`
	HelperPath  string `json:"helperPath,omitempty"`
}

// SystemAPI 暴露只读系统与组件就绪信息。
type SystemAPI struct {
	version string
}

func NewSystemAPI(version string) *SystemAPI {
	return &SystemAPI{version: version}
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
