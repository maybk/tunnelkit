# 构建与打包配置 (`build`)

管理 Wails 应用元信息、三端桌面构建与打包任务、应用图标资源及官方模板预留目录。

## 目录结构

- `config.yml`：应用元信息与版本号（`info.version`）唯一权威来源。
- `Taskfile.yml`：跨平台公共构建、macOS 图标编译与版本资产同步任务。
- `appicon.png`：macOS 应用图标源文件（Apple `824/1024` 安全区）。
- `appicon.icon/`：macOS Icon Composer 分层图标工程。
- `darwin/`：macOS `.app` 应用包、Universal Binary、`Info.plist`、代码签名与 `.dmg` 镜像打包配置。
- `windows/`：Windows 多尺寸图标、资源清单、应用清单与 NSIS 安装器打包配置。
- `linux/`：Linux `.desktop` 启动器、`hicolor` 图标主题、AppImage 与 `nfpm`（DEB / RPM / Arch）打包配置。
- `android/`、`ios/`、`docker/`：Wails 官方模板预留目录。

## 开发规范

- **单一版本源**：应用版本号统一维护在 `config.yml` 的 `info.version` 中，通过 `wails3 task common:update:build-assets` 同步至各平台清单。
- **模板对齐**：保留 Wails 官方模板预留目录与任务，仅在 `darwin/`、`windows/`、`linux/` 桌面任务中扩展 `tunnelkit-helper` 与 `wintun.dll` 打包流程。
- **图标分平台维护**：`generate:icons` 仅编译 macOS 图标产物并显式置空 `-windowsfilename`，避免覆盖 Windows 满幅图标。

## 验证

```bash
wails3 build
wails3 package
```
