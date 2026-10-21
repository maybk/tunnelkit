# 开发与构建

## 环境要求

| 组件 | 要求 |
| --- | --- |
| Go | `>= 1.26.0` |
| Wails CLI | Wails v3，版本与 `go.mod` 及 `frontend/package.json` 中的 `@wailsio/runtime` 保持一致 |
| Node.js / npm | Node.js `>= 22`，使用 `npm` 与 `frontend/package-lock.json` |
| 操作系统 | macOS `12.0+`（需 Xcode Command Line Tools）、Windows `10/11` 或 Linux（需 `GTK4` / `WebKitGTK 6.0` 与 `iproute2`） |

Wails v3 CLI 安装说明见 [Wails v3 官方文档](https://v3.wails.io/quick-start/installation/)。

## 本地开发

首次检出仓库后安装前端依赖：

```bash
npm --prefix frontend ci
```

在仓库根目录启动完整桌面开发环境：

```bash
wails3 dev
```

`wails3 dev` 会自动构建主程序与 `bin/tunnelkit-helper`、启动前端开发服务器并拉起桌面窗口。

如需单独编译独立内核 CLI 进行命令行调试，可执行：

```bash
go build -o bin/fortivpn ./fortivpn/cmd/fortivpn
```

## 格式、分析与测试

常规完整检查：

```bash
npm --prefix frontend run format:check
npm --prefix frontend run lint:check
npm --prefix frontend run test -- --run
npm --prefix frontend run build
go vet ./...
go test -race ./...
bash scripts/test_extract_pending_version.sh
bash scripts/test_extract_release_notes.sh
bash scripts/test_ci_config.sh
bash scripts/test_rolling_release_config.sh
git diff --check
```

测试套件包含 `fortivpn/fortitest` 内存假网关端到端测试、PPP 状态机测试、本地 IPC 与 `tunnelkit-helper` 生命周期测试、Vue 组件测试以及 CI/Release 工作流配置契约测试，均在临时目录与非特权环境中隔离运行。

## 绑定生成与桌面构建

当 `internal/api` 的导出方法或数据结构发生变化时，重新生成前端 TypeScript 绑定：

```bash
wails3 generate bindings -ts
```

构建当前宿主机架构的生产二进制或应用包：

```bash
wails3 build
wails3 package
```

### 按架构独立构建与打包（macOS / Windows）

- **macOS（支持 `arm64`、`amd64` 独立包与 `universal` 通用包）**：
  ```bash
  wails3 task darwin:package:arm64
  wails3 task darwin:package:amd64
  wails3 task darwin:package:universal
  ```

- **Windows（支持 `amd64` 与 `arm64` 独立构建及安装器）**：
  ```bash
  pwsh ./scripts/prepare_windows_wintun.ps1 -OutputDirectory ./bin
  wails3 task windows:build:amd64
  wails3 task windows:build:arm64
  wails3 task windows:package:amd64
  wails3 task windows:package:arm64
  ```

## 发布

本地打包、Rolling 预发布与版本标签自动发布流程见[发布指南](./release.md)。

## 交付检查

1. **后端与内核改动**：通过 `go test -race ./...` 与 `go vet ./...`。
2. **前端改动**：通过 `npm --prefix frontend run format:check`、`lint:check`、`test -- --run` 与 `build`。
3. **绑定、窗口或构建改动**：运行 `wails3 build` 确认主程序、`tunnelkit-helper` 与内嵌前端资源完整编译通过。
4. **CI 与发布配置改动**：运行 `scripts/test_*.sh` 四项契约测试。
5. **配置、目录、命令或用户可见变化**：同步更新 `README.md`、`docs/guide/*` 或 `CHANGELOG.md` 顶部的 `未发布` 段落。
6. **工作区检查**：运行 `git diff --check`，确认无多余空白、临时文件或调试残留。
