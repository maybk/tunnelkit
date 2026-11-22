# 发布指南

TunnelKit 的 Rolling 预发布与正式版本发布已通过 GitHub Actions 自动化编排；包管理器清单由 [`maybk/homebrew-tap`](https://github.com/maybk/homebrew-tap) 统一维护与同步。

## 相关文件

- [`ci.yml`](../../.github/workflows/ci.yml)：`main` 分支推送与 Pull Request 的格式、静态分析、竞态测试与构建契约门禁，同时供发布工作流复用
- [`release-assets.yml`](../../.github/workflows/release-assets.yml)：正式版与 Rolling 共用的跨平台打包流水线（macOS / Windows / Linux）
- [`rolling.yml`](../../.github/workflows/rolling.yml)：每日定时（北京时间 `02:17`）检查 `main` 分支未发布增量提交并更新 `rolling` 预发布，同时支持手动按需触发
- [`release.yml`](../../.github/workflows/release.yml)：`v*` 版本标签触发的正式 GitHub Release 发布与过期 Rolling 清理
- [`prepare_windows_wintun.ps1`](../../scripts/prepare_windows_wintun.ps1)：下载并按固定 SHA-256 校验官方 Wintun 驱动包（`amd64` / `arm64`）
- [`smoke_windows_helper.ps1`](../../scripts/smoke_windows_helper.ps1)：静默安装与卸载 Windows 安装包，校验系统服务与本地 IPC 会话
- [`package_macos_dmg.sh`](../../scripts/package_macos_dmg.sh) 与 [`build/darwin/dmg/settings.py`](../../build/darwin/dmg/settings.py)：使用 `dmgbuild` 直接写入 `.DS_Store`、`Applications` 拖拽快捷方式、Retina `@1x/@2x` 引导背景图与卷标图标
- [`extract_pending_version.sh`](../../scripts/extract_pending_version.sh)：读取并校验 `CHANGELOG.md` 顶部唯一的待发布版本
- [`extract_release_notes.sh`](../../scripts/extract_release_notes.sh)：从 `CHANGELOG.md` 提取指定版本的更新内容
- [`render_release_notes.sh`](../../scripts/render_release_notes.sh)：渲染正式版与 Rolling 预览版的 GitHub Release 说明与下载表
- [`prepare_release.sh`](../../scripts/prepare_release.sh)：一键同步 `build/config.yml` 与 `CHANGELOG.md` 版本段落
- [`build/config.yml`](../../build/config.yml)：应用版本号唯一权威来源（`info.version`）
- [`CHANGELOG.md`](../../CHANGELOG.md)：正式版本与待发布版本更新记录

## 版本号管理

应用版本号统一在 `build/config.yml` 的 `info.version` 中维护：

- 桌面进程启动时读取内嵌的 `build/config.yml` 版本号；特权辅助程序 `tunnelkit-helper` 在全平台构建时注入同一版本号，支持 `--version` 查询。
- 跨平台发布打包时自动运行 `wails3 task common:update:build-assets`，将 `build/config.yml` 中的版本号同步至 macOS `Info.plist`、Windows `info.json` / `wails_tools.nsh` / `manifest`、Linux `nfpm.yaml` 等平台清单。

## 本地打包构建

### macOS（支持 `arm64`、`amd64` 独立包与 `universal` 通用包）

```bash
# 打包 Apple Silicon (arm64) 独立应用包 -> bin/tunnelkit-darwin-arm64.app
wails3 task darwin:package:arm64

# 打包 Intel (amd64 / x86_64) 独立应用包 -> bin/tunnelkit-darwin-amd64.app
wails3 task darwin:package:amd64

# 打包双架构合一通用包 (Universal Binary) -> bin/tunnelkit.app
wails3 task darwin:package:universal
```

构建完成后可通过 `lipo` 验证主程序与内置辅助程序的架构：

```bash
lipo -archs bin/tunnelkit.app/Contents/MacOS/tunnelkit
lipo -archs bin/tunnelkit.app/Contents/MacOS/tunnelkit-helper
```

本地打包默认对 `Contents/MacOS/tunnelkit-helper` 与 `.app` 执行 ad-hoc 签名。未经 Apple Developer ID 签名与公证的分发包在首次下载打开时，可执行以下命令移除隔离属性：

```bash
xattr -dr com.apple.quarantine "/Applications/TunnelKit.app"
```

### Windows（支持 `amd64` 与 `arm64` 独立构建及安装器）

```bash
# 准备并校验官方 Wintun DLL (bin/wintun-amd64.dll 与 bin/wintun-arm64.dll)
pwsh ./scripts/prepare_windows_wintun.ps1 -OutputDirectory ./bin

# 分别编译或打包 Windows amd64 与 arm64
wails3 task windows:build:amd64
wails3 task windows:build:arm64
wails3 task windows:package:amd64
wails3 task windows:package:arm64
```

### Linux（支持 `amd64` 与 `arm64`）

```bash
# 编译当前宿主架构的 bin/tunnelkit 与 bin/tunnelkit-helper
wails3 task linux:build

# 生成带依赖声明（libgtk-4-1 / libwebkitgtk-6.0-4 / iproute2）的 .deb 安装包 -> bin/tunnelkit.deb
wails3 task linux:create:deb

# 生成带依赖声明（gtk4 / webkitgtk6.0 / iproute）的 .rpm 安装包 -> bin/tunnelkit.rpm
wails3 task linux:create:rpm
```

## 自动化发布

普通 CI 同时作为 Rolling 和正式发布的自动质量门禁：

- `ci.yml` 在 `main` push 和 Pull Request 上运行，也可由 Release 与 Rolling 复用；仅具备 `contents: read` 只读权限。
- `release-assets.yml` 按平台与架构拆分为 7 个并行 Job，统一校验 `tunnelkit-helper --version` 与目标版本一致：
  - macOS（`macos-latest`）：并行构建 `arm64` / `amd64` / `universal` DMG，并在 `arm64` 上运行静态分析、竞态测试与已安装特权辅助程序冒烟测试。
  - Windows（`windows-latest` / `windows-11-arm`）：构建 `amd64` / `arm64` 便携包与 NSIS 安装器，运行测试套件（`amd64` 启用 `-race`）与安装器及服务冒烟测试。
  - Linux（`ubuntu-24.04` / `ubuntu-24.04-arm`）：构建 `amd64` / `arm64` 压缩包、`.deb` 与 `.rpm`，校验依赖与文件列表，并运行竞态测试、`.deb` 安装冒烟测试及 `xvfb` 无头 GUI 冒烟测试。
- `rolling.yml` 每日 `02:17`（UTC+8）定时运行，也支持在 `main` 分支手动触发（可选 `force` 强制重打）。工作流锚定 `origin/main` 最新提交并复用已通过的 push CI（无记录时先运行 `ci.yml`），基于 `CHANGELOG.md` 顶部的 `## [x.y.z] - 未发布` 将内嵌版本构建为 `<version>-pre` 并发布至 `v<version>-pre`（资产命名为 `tunnelkit-<version>-rolling-<12位SHA>-<platform>`）。若目标提交已打正式标签、待发布段落为空或完整预发布已存在则跳过；若预发布残缺则自动重建修复。
- `release.yml` 仅由 `vMAJOR.MINOR.PATCH` 标签触发（排除 `*-*` 预发布标签），要求标签版本与 `build/config.yml` 及带日期的 `CHANGELOG.md` 版本段落一致，且标签提交已包含在 `main` 分支中。
- 自动发布产物固定为 13 个平台安装包加 1 个 `SHA256SUMS.txt`（共 14 个文件）：
  - `tunnelkit-<version>-macos-universal.dmg`
  - `tunnelkit-<version>-macos-arm64.dmg`
  - `tunnelkit-<version>-macos-amd64.dmg`
  - `tunnelkit-<version>-windows-amd64-setup.exe`
  - `tunnelkit-<version>-windows-amd64.zip`
  - `tunnelkit-<version>-windows-arm64-setup.exe`
  - `tunnelkit-<version>-windows-arm64.zip`
  - `tunnelkit-<version>-linux-amd64.tar.gz`
  - `tunnelkit-<version>-linux-amd64.deb`
  - `tunnelkit-<version>-linux-amd64.rpm`
  - `tunnelkit-<version>-linux-arm64.tar.gz`
  - `tunnelkit-<version>-linux-arm64.deb`
  - `tunnelkit-<version>-linux-arm64.rpm`
  - `SHA256SUMS.txt`
- Release 说明由 `render_release_notes.sh` 生成，展示更新内容、系统版本与常用安装包（Windows Setup、macOS Apple Silicon/Intel DMG、Linux DEB/RPM）下载徽章；通用包、便携包与 `SHA256SUMS.txt` 位于底部 `Assets`。
- 正式版与 Rolling 均先生成并自校验 `SHA256SUMS.txt`，再上传至草稿 Release 验收全部 14 个资产后公开（Rolling 先在临时草稿验收完整再替换线上预发布）；正式版公开后自动清理指向该提交或其祖先的 Rolling 预发布与标签。
- 所有第三方 GitHub Actions 均锁定到完整 40 位 commit SHA，并由 Dependabot 按周分组更新。

## 发布新版本

1. 更新 `build/config.yml` 中的 `info.version`，将 `CHANGELOG.md` 中目标版本的 `未发布` 改为正式日期（`YYYY-MM-DD`）并在其上方新增下一版本的 `## [x.y.z] - 未发布` 段落（也可直接执行辅助脚本一键完成）：
   ```bash
   bash scripts/prepare_release.sh
   ```
2. 运行全量本地检查：
   ```bash
   pnpm --dir frontend run format:check
   pnpm --dir frontend run lint:check
   pnpm --dir frontend run test -- --run
   pnpm --dir frontend run build
   go vet ./...
   go test -race ./...
   bash scripts/test_extract_pending_version.sh
   bash scripts/test_extract_release_notes.sh
   bash scripts/test_prepare_release.sh
   bash scripts/test_ci_config.sh
   bash scripts/test_rolling_release_config.sh
   git diff --check
   ```
3. 提交版本变更、创建附注标签并原子推送 `main` 与标签：
   ```bash
   version="$(sed -n 's/^[[:space:]]*version:[[:space:]]*"\([^"]*\)".*/\1/p' build/config.yml | head -n 1)"
   git commit -am "chore(release): v$version"
   git tag -a "v$version" -m "TunnelKit v$version"
   git push --atomic origin main "v$version"
   ```
4. 正式版公开后，[`maybk/homebrew-tap`](https://github.com/maybk/homebrew-tap) 会按计划自动同步 Homebrew Cask、Scoop Bucket 与 WinGet 清单；如需立即同步，可执行：
   ```bash
   gh workflow run sync.yml -R maybk/homebrew-tap
   ```
