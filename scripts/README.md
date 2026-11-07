# 构建与发版脚本 (`scripts`)

存放跨平台驱动资产准备、安装包制作、版本与发版说明提取、包管理器清单渲染及配套自检脚本。

## 目录结构

- `extract_pending_version.sh`：从 `CHANGELOG.md` 顶部提取待发布版本号（`## [X.Y.Z] - 未发布`）。
- `extract_release_notes.sh`：从 `CHANGELOG.md` 提取指定正式版本或待发布版本的更新说明正文。
- `prepare_release.sh`：发版前统一更新 `build/config.yml` 版本号与 `CHANGELOG.md` 正式版本标题及日期。
- `package_macos_dmg.sh`：基于 `dmgbuild` 与 `build/darwin/dmg/` 资产打包 macOS `.dmg` 安装镜像。
- `prepare_windows_wintun.ps1`：下载固定版本官方 Wintun 驱动压缩包、校验 SHA-256 并按目标架构提取 `wintun.dll`。
- `render_package_manifests.sh`：根据发版版本号与 `SHA256SUMS.txt` 渲染并校验 Homebrew Cask、Scoop 与 WinGet 清单文件。
- `test_extract_pending_version.sh`：`extract_pending_version.sh` 的边界回归测试。
- `test_extract_release_notes.sh`：`extract_release_notes.sh` 与 `CHANGELOG.md` 格式守卫测试。
- `test_prepare_release.sh`：`prepare_release.sh` 的版本同步与幂等性回归测试。
- `test_package_manifests.sh`：`render_package_manifests.sh` 的清单渲染回归测试。
- `test_ci_config.sh`：校验 `.github/workflows/ci.yml`、锁文件唯一性与依赖源安全约束。
- `test_rolling_release_config.sh`：校验滚动预发布与正式发版工作流编排契约。

## 开发规范

- **单一版本源**：版本号权威来源固定为 `build/config.yml` 的 `info.version`；未明确执行正式发布前，`CHANGELOG.md` 顶部必须保持 `## [X.Y.Z] - 未发布` 标题。
- **脚本健壮性**：所有 Shell 脚本须开启 `set -euo pipefail`，测试脚本使用 `mktemp -d` 并在 `EXIT` trap 中清理临时目录，不污染工作区。

## 验证

```bash
bash scripts/test_extract_pending_version.sh
bash scripts/test_extract_release_notes.sh
bash scripts/test_prepare_release.sh
bash scripts/test_package_manifests.sh
bash scripts/test_ci_config.sh
bash scripts/test_rolling_release_config.sh
```
