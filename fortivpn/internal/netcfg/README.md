# 路由、DNS 与异常恢复 (`internal/netcfg`)

本包负责在隧道建立后配置宿主路由表与 DNS，并通过持久化恢复日志确保进程异常退出后仍能幂等还原宿主网络。

## 职责

- [netcfg.go](netcfg.go) / [plan.go](plan.go)：路由与 DNS 变更规划，自动计算 VPN 网关 `/32` 主机保护路由（防止默认路由覆盖导致隧道自环）。
- [journal.go](journal.go)：变更前先将回滚条目持久化至 `journal.json`；正常断开或下次启动调用 `Recover` 时逆序幂等回滚。
- [darwin.go](darwin.go) / [dns_darwin.go](dns_darwin.go)：macOS 通过 `route` 管理路由，通过 `/etc/resolver/<domain>` 与 `scutil` 配置分流或全局 DNS。
- [windows.go](windows.go) / [dns_windows.go](dns_windows.go)：Windows 通过 `route.exe` / `netsh` 配置接口地址、路由与 DNS，支持 NRPT 域名分流策略。
- [linux.go](linux.go) / [linux_dns.go](linux_dns.go)：Linux 通过 `ip addr` / `ip route` 与 `resolvectl`（或 `/etc/resolv.conf` 备份）管理路由和 DNS。

## 验证

```bash
cd fortivpn && go test -race ./internal/netcfg/...
```
