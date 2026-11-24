# 宿主路由、DNS 与异常恢复 (`fortivpn/internal/netcfg`)

在隧道建立后配置三端宿主路由表与 DNS，并通过持久化写前恢复日志（WAL）确保进程异常退出后仍能按 LIFO 逆序幂等还原宿主网络。

## 目录结构

- `netcfg.go`：目标网络配置模型、宿主路由快照、计划步骤与已应用句柄接口。
- `netcfg_darwin.go`：macOS 原生网络配置入口。
- `route_darwin.go`：macOS 原生 `PF_ROUTE` 路由增删与路由表查询实现。
- `route_darwin_other.go`：非 macOS 平台的原生路由后端占位实现。
- `netcfg_darwin_plan.go`：macOS 网卡配置与可注入路由编排。
- `dns_darwin_plan.go`：macOS `scutil` DNS 配置、唤醒核验与缓存刷新。
- `netcfg_linux.go`：Linux 原生网络配置入口。
- `netlink_linux.go`：Linux 原生 Netlink 接口与路由实现。
- `netlink_other.go`：非 Linux 平台的原生 Netlink 后端占位实现。
- `netcfg_linux_plan.go`：Linux 可注入 Netlink 与命令回退编排。
- `dns_linux_plan.go`：Linux `resolvectl` / `resolvconf` DNS 配置、唤醒核验与回退。
- `orchestrator_unix_plan.go`：macOS / Linux 共享会话编排、切网路由刷新、DNS 核验与回滚。
- `netcfg_windows.go`：Windows 原生网络配置入口与 `winipcfg` 适配器实现。
- `netcfg_windows_plan.go`：Windows 网络会话应用、差量更新、唤醒核验、关闭与 WAL 回滚编排。
- `dns_windows.go`：Windows 接口 DNS 配置、注册表 NRPT 域名分流策略与缓存刷新。
- `netcfg_unsupported.go`：不支持平台入口。
- `plan.go`：路由清洗去重、排除网段差集计算、默认路由拆分、会话期路由差量与配置步骤规划。
- `journal.go`：写前恢复日志（`netcfg-journal.json`）原子持久化、LIFO 逆序回滚及失败条目保留重试。
- `journal_codec.go`：版本化日志编解码与旧数组格式兼容。
- `journal_validation.go`：校验恢复条目数量上限与平台资源键白名单。
- `journal_import.go`：校验旧恢复数据的平台资源范围，去重合并到受保护 WAL 后再执行恢复。
- `journal_routes.go`：三端共用的主机路由恢复条目构造与写前记录辅助。
- `runner.go`：可注入的外部系统命令执行器及路由幂等错误判定辅助。

## 开发规范

- **纯 Go 计划与原生调用分层**：原生入口与系统调用留在 `*_darwin.go`、`*_linux.go`、`*_windows.go` 及 `*_other.go` 占位；纯 Go 编排、解析与会话状态机放在无构建标签限制的 `*_plan.go`，在任意宿主 OS 直接运行单元测试。
- **WAL 先写后改**：任何跨进程留存的宿主路由或 DNS 变更，必须先原子写入受保护目录中的 `netcfg-journal.json` 再执行系统变更；回滚按 LIFO 逆序幂等执行，失败条目保留供下次重试。
- **网关主机路由先行**：下发隧道子网路由前，必须先通过原物理默认网关固定 VPN 网关 IP 的 `/32` 主机路由，防止加密隧道流量自环。

## 验证

```bash
go test -race ./fortivpn/internal/netcfg/...
```
