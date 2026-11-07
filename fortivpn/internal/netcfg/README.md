# 宿主路由、DNS 与异常恢复 (`fortivpn/internal/netcfg`)

在隧道建立后配置三端宿主路由表与 DNS，并通过持久化写前恢复日志（WAL）确保进程异常退出后仍能按 LIFO 逆序幂等还原宿主网络。

## 目录结构

- `netcfg.go`：目标网络配置模型、宿主路由快照、计划步骤与已应用句柄接口。
- `plan.go`：路由清洗去重、排除网段差集计算、默认路由拆分、会话期路由差量与配置步骤规划。
- `journal.go`：写前恢复日志（`netcfg-journal.json`）原子持久化、LIFO 逆序回滚及失败条目保留重试。
- `journal_validation.go`：严格校验恢复日志 JSON 结构、条目数量上限与平台资源键白名单。
- `runner.go`：可注入的外部系统命令执行器及路由幂等错误判定辅助。
- `darwin.go`：macOS 原生网络配置实现，驱动 `ifconfig` 与 `route` 管理接口地址、网关保护路由及隧道子网路由。
- `dns_darwin.go`：macOS 基于 `scutil` 动态存储字典配置全局或按域名后缀匹配的分流 DNS。
- `windows.go`：Windows 原生网络配置实现，配置 Wintun 接口地址、网关主机路由与子网路由。
- `dns_windows.go`：Windows 配置接口 DNS 服务器，并通过注册表下发与清理 NRPT 域名分流策略。
- `linux.go`：Linux 原生网络配置实现，通过 `ip addr` 与 `ip route` 管理接口地址、网关保护路由与子网路由。
- `dns_linux.go`：Linux 优先通过 `resolvectl` 配置接口 DNS 与路由域，无 `resolvectl` 时回退至 `resolvconf`。
- `other.go`：非支持平台下的网络配置与恢复空实现。

## 开发规范

- **WAL 先写后改**：任何需要跨进程恢复的宿主路由或 DNS 变更，必须先调用 `Journal.Record` 持久化至受保护目录中的 `netcfg-journal.json` 再执行系统变更；回滚时按 LIFO 逆序幂等执行，失败条目保留在日志中供下次重试。
- **网关主机路由先行**：下发隧道子网路由前，必须先通过原物理默认网关固定 VPN 网关 IP 的 `/32` 主机路由，防止加密隧道流量自环。

## 验证

```bash
go test -race ./fortivpn/internal/netcfg/...
```
