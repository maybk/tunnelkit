# 开发与构建

## 环境要求

| 组件 | 要求 |
| --- | --- |
| Go | `>= 1.27.4` |
| Wails CLI | Wails v3，版本与 `go.mod` 及 `frontend/package.json` 中的 `@wailsio/runtime` 保持一致 |
| Node.js / pnpm | Node.js `>= 26.5`，使用 `pnpm` 与 `frontend/pnpm-lock.yaml` |
| 静态与漏洞扫描（可选） | `golangci-lint` v2、`govulncheck`（未预装时由 `wails3 task lint` / `check` 自动通过 `go run` 执行） |
| 操作系统 | macOS `12.0+`（需 Xcode Command Line Tools）、Windows `10/11` 或 Linux（需 `GTK4` / `WebKitGTK 6.0` 与 `iproute2`） |

Wails v3 CLI 安装说明见 [Wails v3 官方文档](https://v3.wails.io/quick-start/installation/)。

## 本地开发

首次检出仓库后安装锁定版本的前端依赖：

```bash
pnpm --dir frontend install --frozen-lockfile
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

日常检查统一通过 `wails3 task` 执行（本机已装 `go-task` 时亦可直接用 `task`）：

```bash
wails3 task fmt              # 格式化全部 Go 与前端源码
wails3 task lint             # 前端检查 + go vet + golangci-lint v2 + govulncheck
wails3 task test             # 前端单测 + Go 竞态单测
wails3 task check            # 完整质量门禁（格式、扫描、单测、前端构建、发布契约与空白检查）
```

测试套件包含 `fortivpn/fortitest` 内存假网关端到端测试、PPP 状态机测试、三端网络配置测试、本地 IPC 与 `tunnelkit-helper` 生命周期测试、Vue 组件测试以及 CI/Release 工作流配置契约测试，均在临时目录与非特权环境中隔离运行。

## 平台文件命名

文件名表达包内职责，同一职责共享基名；原生实现按平台使用 `_darwin.go`、`_linux.go`、`_windows.go` 后缀，纯 Go 共用模块直接用简短职责名。

平台差异按语义选用后缀：`_unix.go` 为 Unix 共用实现（纯 POSIX 用 `unix` 标签，仅支持 macOS / Linux 时用 `darwin || linux`），`_other.go` 为默认或降级实现，`_unsupported.go` 为不支持平台入口。需跨平台单测的单端计划或解析模块使用 `<role>_<os>_plan.go` 或 `<role>_<os>_parse.go`，避免触发 Go 原生 `*_GOOS.go` 过滤。

## 绑定生成与桌面构建

当 `internal/api` 的导出方法或数据结构发生变化时，重新生成前端 TypeScript 绑定：

```bash
wails3 generate bindings -clean=true -ts
```

构建当前宿主机架构的生产二进制或应用包：

```bash
wails3 build
wails3 package
```

### 按架构独立构建与打包（macOS / Windows / Linux）

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

- **Linux（支持 `amd64` 与 `arm64` 二进制及 `.deb` / `.rpm` 安装包）**：
  ```bash
  wails3 task linux:build
  wails3 task linux:create:deb
  wails3 task linux:create:rpm
  ```

## 发布

本地打包、Rolling 预发布与版本标签自动发布流程见[发布指南](./release.md)。

## 交付检查

1. **架构与文件边界红线**：确认 `fortivpn/**` 未导入 `tunnelkit/internal/**` 或 Wails，桌面 GUI 未导入 `fortivpn/engine`，`internal/redact` 零内部依赖，`internal/privilege` 零内部与 Wails 依赖，`internal/storage` 未导入上层业务包或 Wails，`internal/api` 未直接执行文件 I/O（`<= 160` 行）；手写 Go 文件满足单文件行数上限（源码 `<= 500` 行、测试 `<= 800` 行）与职责命名门禁。
2. **全量质量门禁**：运行 `wails3 task check`（覆盖格式、静态与漏洞扫描、前后端单测、前端构建、发布契约与空白检查）。
3. **绑定、窗口或构建改动**：运行 `wails3 build` 确认主程序、`tunnelkit-helper` 与内嵌前端资源完整编译通过。
4. **配置、目录、命令或用户可见变化**：同步更新 `README.md`、`docs/guide/*`、对应目录 `README.md` 或 `CHANGELOG.md` 顶部的 `未发布` 段落。
