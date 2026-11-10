# 内核私有子包 (`fortivpn/internal`)

承载 `fortivpn` 控制面协议、PPP 状态机、跨平台 TUN 虚拟网卡与宿主网络配置的内部实现子包，仅对 `fortivpn` 模块内部可见。

## 目录结构

- `sslvpn/`：FortiGate HTTPS 控制面认证、隧道 XML 解析、裸 TLS 连接劫持与 `0x5050` 6 字节帧编解码。
- `ppp/`：跑在 `0x5050` 隧道帧之上的 RFC 1661 PPP 客户端状态机与链路保活。
- `netdev/`：macOS `utun`、Windows Wintun 与 Linux `/dev/net/tun` 三端虚拟网卡适配。
- `netcfg/`：三端宿主路由表、分流与全局 DNS 配置以及基于 `netcfg-journal.json` 的 LIFO 逆序异常恢复。

## 开发规范

- **依赖边界**：各子包仅允许依赖 `fortivpn` 根包契约与同层基础子包，严禁反向依赖 `fortivpn/engine`、桌面 `internal/*` 或 Wails。
- **三端对等与可移植测试**：涉及外部网络命令或平台状态变更的子包须通过可注入命令执行器覆盖三端计划与回滚单测。

## 验证

```bash
go test -race ./fortivpn/internal/...
```
