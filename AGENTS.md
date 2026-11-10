# TunnelKit

TunnelKit 是基于 Wails v3、Go 与 Vue 3 + TypeScript 的跨平台（macOS / Windows / Linux）Fortinet SSL-VPN 桌面与菜单栏客户端，内置纯 Go 协议内核 `fortivpn` 与按会话运行的特权辅助程序 `tunnelkit-helper`。

## 架构与模块边界

依赖方向固定为：

- 桌面进程：`Vue 前端 -> Wails bindings -> internal/api -> internal/vpn | internal/profile | internal/update -> internal/helperclient | internal/ipc | internal/privilege | internal/credentials | internal/storage | fortivpn`
- 前端内部：`views / layouts / components -> composables / stores -> api -> Wails bindings / runtime`；展示、校验、领域、工具、常量与类型为共享层，不反向依赖 UI 或宿主（仅 `frontend/src/api/` 允许导入 `@bindings` 与 `@wailsio/runtime`；`frontend/src/components/ui/` 同步自 `shadcn-vue` 官方模板）
- 特权辅助进程：`cmd/tunnelkit-helper -> internal/helper -> internal/ipc | internal/privilege | internal/redact | fortivpn/engine | fortivpn`
- 独立协议内核：`fortivpn/**` **严禁**引用 `tunnelkit/internal/**` 或 Wails，可随时独立拆出为单独仓库；桌面主进程**严禁**导入 `fortivpn/engine`。

各模块职责：

- `fortivpn`：纯 Go Fortinet SSL-VPN 协议内核与独立 CLI。
- `cmd/tunnelkit-helper` 与 `internal/helper`：单次 VPN 会话生命周期运行的特权辅助程序，监听本机 IPC 通道，校验调用方身份与一次性令牌后驱动 `fortivpn/engine`。
- `internal/ipc` 与 `internal/helperclient`：本地 IPC 传输（Unix Domain Socket / Windows Named Pipe）、加密会话票据与辅助进程按需启动器。
- `internal/privilege`：三端特权组件的受保护安装、完整性与权限校验、系统授权、服务注册与运行路径。
- `internal/credentials`：平台主密钥保护与 AES-256-GCM 凭据加解密，供连接密码与会话票据复用。
- `internal/vpn`：桌面端连接状态机、交互挑战转发、会话接管、更新维护门禁与网络恢复。
- `internal/profile`：连接配置增删改查、字段校验与密码加密存储。
- `internal/storage`：统一管理 `~/.tunnelkit` 目录路径与原子文件写入。
- `internal/redact`：日志写入与读取共用的凭据脱敏，仅依赖标准库。
- `internal/update`：多源版本检查、发布地址白名单校验、语义化版本比较与三端原子自更新。
- `internal/api`：Wails 服务薄适配层，仅做参数转发与跨包编排；领域逻辑与 I/O 实现下沉至对应 `internal/*` 子包。
- `main.go` 与 `internal/app`：应用装配、窗口与标题栏配置、托盘常驻、安全退出、自更新协调及 `vpn:state` 事件推送。

## 常用命令

在项目根目录执行：

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run format:check
pnpm --dir frontend run lint:check
pnpm --dir frontend run test -- --run
pnpm --dir frontend run build
go vet ./...
go test -race ./...
bash scripts/test_extract_pending_version.sh
bash scripts/test_extract_release_notes.sh
bash scripts/test_prepare_release.sh
bash scripts/test_package_manifests.sh
bash scripts/test_ci_config.sh
bash scripts/test_rolling_release_config.sh
```

## 修改与安全规则

1. 未经明确要求，不执行 `git commit`、`git push`、切换分支或改写 Git 历史；保留现有改动，不顺带重构无关代码。
2. 密码、Cookie、Token、OTP 验证码与密钥禁止硬编码或写入日志、源码及错误提示；密码与认证令牌仅通过本地 IPC 通道传输，严禁落地为明文临时文件或命令行参数。
3. `profiles.json` 仅保存非敏感字段；连接密码与运行期会话重连令牌经 `internal/credentials` 加密后分别存入 `secrets.json` 与 `run/session.json`，主密钥保存在 `~/.tunnelkit/secret.key`。
4. 辅助程序日志必须经过 `internal/helper` 脱敏器，再通过已鉴权 IPC 交给非特权桌面进程落盘。
5. 网络路由与 DNS 修改必须先持久化写入系统保护目录中的 `run/<uid-or-sid>/netcfg-journal.json`（WAL）再执行系统变更，确保进程异常退出后能按 LIFO 逆序幂等恢复宿主网络。
6. `fortivpn` 包必须保持零外部业务耦合，禁止在 `fortivpn/**` 中导入 `tunnelkit/internal/**` 或 Wails；桌面 GUI 进程禁止导入 `tunnelkit/fortivpn/engine`。
7. 应用版本号以 `build/config.yml` 的 `info.version` 为唯一权威来源（运行期由 `main.go` 内嵌读取，打包期通过 `wails3 task common:update:build-assets` 同步至平台清单）。
8. 修复缺陷需先补充复现测试，完成后运行 `go test -race ./...` 与 `go vet ./...`；涉及 Wails 绑定、窗口或构建配置变更时须运行 `wails3 build`。

## 文档维护

- `README.md` 只保留项目入口、功能特性、快速开始和文档链接。
- 稳定架构、配置、开发和发版说明放在 `docs/guide/`，排错过程和阶段性方案不写入长期指南。
- 目录 `README.md` 遵循单层局部索引原则：仅列出当前目录的直接文件与直接子目录，不跨层展开子目录内部文件；核心二级子目录说明职责、目录结构和开发规范；专属验证流程按需补充，前端通用验证命令集中维护在入口 README 与开发、测试指南。
- 修改配置规则、目录职责、安全边界、命令或构建产物时，同步更新 `docs/guide/*` 与对应目录 `README.md`。
- 每次完成用户可见的新增、变更或修复时，同步更新 `CHANGELOG.md` 顶部的待发布版本（`## [X.Y.Z] - 未发布`）；只记录相对上一正式版的最终净变化，不记录中间调试过程。
- 未明确要求正式发布时，只维护待发布版本，不修改正式版本号、正式日期和已发布版本，也不创建标签或 Release。
