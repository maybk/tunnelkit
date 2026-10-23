# 发布指南

TunnelKit 的 Rolling 预发布与正式版本发布已通过 GitHub Actions 自动化编排。

## 相关文件

- [`ci.yml`](../../.github/workflows/ci.yml)：`main` 分支推送与 Pull Request 的格式、静态分析、竞态测试与构建契约门禁，同时供发布工作流复用
- [`release-assets.yml`](../../.github/workflows/release-assets.yml)：正式版与 Rolling 共用的跨平台打包流水线（macOS / Windows / Linux）
- [`rolling.yml`](../../.github/workflows/rolling.yml)：`main` 分支通过普通 CI 后自动更新 `rolling` 预发布
- [`release.yml`](../../.github/workflows/release.yml)：`v*` 版本标签触发的正式 GitHub Release 发布与过期 Rolling 清理
- [`prepare_windows_wintun.ps1`](../../scripts/prepare_windows_wintun.ps1)：下载并按固定 SHA-256 校验官方 Wintun 驱动包（`amd64` / `arm64`）
- [`package_macos_dmg.sh`](../../scripts/package_macos_dmg.sh) 与 [`build/darwin/dmg/settings.py`](../../build/darwin/dmg/settings.py)：使用 `dmgbuild` 直接写入 `.DS_Store`、`Applications` 拖拽快捷方式、Retina `@1x/@2x` 引导背景图与卷标图标
- [`extract_pending_version.sh`](../../scripts/extract_pending_version.sh)：读取并校验 `CHANGELOG.md` 顶部唯一的待发布版本
- [`extract_release_notes.sh`](../../scripts/extract_release_notes.sh)：从 `CHANGELOG.md` 提取指定版本的发布说明
- [`build/config.yml`](../../build/config.yml)：应用版本号唯一权威来源（`info.version`）
- [`CHANGELOG.md`](../../CHANGELOG.md)：正式版本与待发布版本更新记录

## 版本号管理

应用版本号统一在 `build/config.yml` 的 `info.version` 中维护：

- 桌面进程启动时直接从内嵌的 `build/config.yml` 读取版本号暴露给前端与「关于」弹窗。
- 跨平台发布打包时自动运行 `wails3 task common:update:build-assets`，将 `build/config.yml` 中的版本号同步至 macOS `Info.plist`、Windows `info.json` / `wails_tools.nsh` / `manifest` 等平台清单。

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
```

## 自动化发布

普通 CI 同时作为 Rolling 和正式发布的自动质量门禁：

- `ci.yml` 在 `main` push 和 Pull Request 上运行，也可由 Release 与手动 Rolling 复用；仅具备 `contents: read` 只读权限。
- `release-assets.yml` 按架构拆分为 7 个并行 Job（macOS 3 个包在 `macos-latest` 并行构建、Windows `amd64` 在 `windows-latest` 构建、Windows `arm64` 在原生 `windows-11-arm` 构建并运行测试套件、Linux `amd64` 在 `ubuntu-24.04` 构建并运行 `xvfb` 无头冒烟测试、Linux `arm64` 在原生 `ubuntu-24.04-arm` 构建并运行测试套件与 `xvfb` 无头冒烟测试）。
- `rolling.yml` 只接受成功完成的 `main` push CI，并使用该次 CI 的精确提交 SHA；公开版本取自 `CHANGELOG.md` 顶部唯一的 `## [x.y.z] - 未发布`，标题为 `TunnelKit <version> Rolling`，资产使用 `tunnelkit-<version>-rolling-<12位SHA>-<platform>` 命名并附带 `SHA256SUMS.txt`。当待发布段落为空时自动跳过 Rolling。
- `release.yml` 仅由 `vMAJOR.MINOR.PATCH` 标签触发，要求标签版本与 `build/config.yml` 及带日期的 `CHANGELOG.md` 版本段落一致，且标签提交已包含在 `main` 分支中。
- 自动发布产物固定为 9 个平台安装包加 1 个 `SHA256SUMS.txt`（共 10 个文件）：
  - `tunnelkit-<version>-macos-universal.dmg`
  - `tunnelkit-<version>-macos-arm64.dmg`
  - `tunnelkit-<version>-macos-amd64.dmg`
  - `tunnelkit-<version>-windows-amd64.zip`
  - `tunnelkit-<version>-windows-arm64.zip`
  - `tunnelkit-<version>-linux-amd64.tar.gz`
  - `tunnelkit-<version>-linux-amd64.deb`
  - `tunnelkit-<version>-linux-arm64.tar.gz`
  - `tunnelkit-<version>-linux-arm64.deb`
  - `SHA256SUMS.txt`
- workflow 先创建 Draft Release，上传并校验全部 10 个资产后才公开为 Latest；正式版成功公开后自动清理指向该提交或其祖先的 `rolling` 预发布与标签。
- 所有第三方 GitHub Actions 均锁定到完整 40 位 commit SHA，并由 Dependabot 按周分组更新。

## 发布新版本

1. 更新 `build/config.yml` 中的 `info.version`。
2. 将 `CHANGELOG.md` 中目标版本的 `未发布` 改为正式日期（`YYYY-MM-DD`），并在其上方新增下一个语义版本的 `## [x.y.z] - 未发布` 段落。
3. 运行全量本地检查并创建版本提交：
   ```bash
   pnpm --dir frontend run format:check
   pnpm --dir frontend run lint:check
   pnpm --dir frontend run test -- --run
   pnpm --dir frontend run build
   go vet ./...
   go test -race ./...
   bash scripts/test_extract_pending_version.sh
   bash scripts/test_extract_release_notes.sh
   bash scripts/test_ci_config.sh
   bash scripts/test_rolling_release_config.sh
   git diff --check
   ```
4. 在同一提交上创建 annotated tag，并原子推送 `main` 与标签：
   ```bash
   version="$(sed -n 's/^[[:space:]]*version:[[:space:]]*"\([^"]*\)".*/\1/p' build/config.yml | head -n 1)"
   git tag -a "v$version" -m "TunnelKit v$version"
   git push --atomic origin main "v$version"
   ```
