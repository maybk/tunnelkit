# TunnelKit

TunnelKit 是基于 Wails v3、Go 与 Vue 3 + TypeScript 的跨平台（macOS / Windows / Linux）Fortinet SSL-VPN 桌面与菜单栏客户端，内置纯 Go 实现的 `fortivpn` 协议内核与按会话运行的后台辅助程序 `tunnelkit-helper`。

## 架构与模块边界

依赖方向固定为：

- 桌面进程：`Vue 前端 -> Wails bindings -> internal/api -> internal/vpn -> internal/helperclient | internal/ipc | internal/profile | internal/storage | fortivpn`
- 特权辅助进程：`cmd/tunnelkit-helper -> internal/helper -> internal/ipc | fortivpn/engine | fortivpn`
- 独立协议内核：`fortivpn/**` **严禁**引用 `tunnelkit/internal/**` 或 Wails，可随时独立拆出为单独仓库。

各模块职责：

- `fortivpn`：纯 Go Fortinet SSL-VPN 协议内核（配置解析、`internal/sslvpn` 控制面、`internal/ppp` 协议栈、`internal/netdev` 平台虚拟网卡、`internal/netcfg` 路由/DNS 配置与恢复、`engine` 会话编排、`fortitest` 内存假网关）。
- `cmd/tunnelkit-helper` 与 `internal/helper`：单次 VPN 会话生命周期运行的特权辅助程序，监听本机 IPC 通道，校验调用方身份与一次性令牌后驱动 `fortivpn/engine`。
- `internal/ipc` 与 `internal/helperclient`：本地 IPC 传输与按需提权启动器。
- `internal/vpn`：桌面端连接状态机、交互式挑战（OTP / FortiToken Push / SAML）转发、会话接管与网络恢复。
- `internal/profile`：配置增删改查、字段校验与密码加密存储。
- `internal/storage`：统一管理 `~/.tunnelkit` 目录路径与原子文件写入。
- `internal/api`：Wails 服务薄适配层，负责参数转发与简单编排。
- `main.go` 与 `internal/app`：应用装配、窗口与标题栏配置、关闭隐藏到菜单栏及 `vpn:state` 事件推送。

## 常用命令

在项目根目录执行：

```bash
npm --prefix frontend ci
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
```

## 修改与安全规则

1. 未经明确要求，不执行 `git commit`、`git push`、切换分支或改写 Git 历史；保留现有改动，不顺带重构无关代码。
2. 密码、Cookie、Token、OTP 验证码与密钥禁止硬编码，也禁止写入日志、源码及错误提示；密码与认证令牌仅通过本地 IPC 通道传输，严禁落地为明文临时文件或命令行参数。
3. `profiles.json` 仅保存非敏感字段；密码加密后存入 `secrets.json`，主密钥保存在 `~/.tunnelkit/secret.key`。
4. 辅助程序日志落地前必须经过 `internal/helper` 脱敏器遮蔽会话凭据、密码与令牌。
5. 网络路由与 DNS 修改必须先持久化写入 `run/netcfg-journal.json` 再执行系统变更，确保进程异常退出后能幂等恢复宿主网络。
6. `fortivpn` 包必须保持零外部业务耦合，禁止在 `fortivpn/**` 中导入 `tunnelkit/internal/**`。
7. 应用版本号在 `build/config.yml`、`internal/app/app.go`、`build/windows/nsis/wails_tools.nsh` 与 `build/linux/nfpm/nfpm.yaml` 中同步维护。
8. 修复缺陷需先补充复现测试，完成后运行 `go test -race ./...` 与 `go vet ./...`；涉及 Wails 绑定、窗口或构建配置变更时须运行 `wails3 build`。

## 文档维护

- `README.md` 只保留项目入口、功能特性、快速开始和文档链接。
- 稳定架构、配置、开发和发版说明放在 `docs/guide/`，排错过程和阶段性方案不写入长期指南。
- 修改配置规则、目录职责、安全边界、命令或构建产物时，同步更新 `docs/guide/*`。
- 每次完成用户可见的新增、变更或修复时，同步更新 `CHANGELOG.md` 顶部的待发布版本（`## [X.Y.Z] - 未发布`）；只记录相对上一正式版的最终净变化，不记录中间调试过程。
- 未明确要求正式发布时，只维护待发布版本，不修改正式版本号、正式日期和已发布版本，也不创建标签或 Release。
