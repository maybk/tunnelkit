# 内核会话编排 (`fortivpn/engine`)

将 HTTPS 控制面认证、PPP 链路协商、跨平台 TUN 虚拟网卡、宿主路由/DNS 配置与双向数据包转发组装为完整的 VPN 会话生命周期。

## 目录结构

- `engine.go`：引擎配置选项、会话运行入口 `Run`、无网卡握手自检 `Check` 与残留网络恢复 `Recover`。
- `recovery.go`：旧版网络恢复数据的内核导入入口，持久化后由 `Recover` 统一回滚。
- `session.go`：单次尝试与自动重连状态机编排、指数退避重试、免重登重连判定及反序优雅清理。
- `link.go`：绑定底层 `sslvpn.TunnelConn` 与 `ppp.Session`，管理控制队列、数据队列及 PPP 协商就绪等待。
- `netcfg.go`：汇总网关下发与用户自定义的路由和 DNS 规则，驱动虚拟网卡打开及宿主网络初次应用或重连差量更新。
- `pump.go`：TUN 设备与 TLS 隧道之间的双向并发数据包转发、LCP Echo 死链探测及流量统计采样。

## 开发规范

- **导入隔离**：仅允许由 `cmd/tunnelkit-helper`、`internal/helper`、`fortivpn/cmd/fortivpn` 及测试加载；桌面 GUI 主进程严禁直接导入本包。
- **反序清理守卫**：无论会话正常取消还是中途报错退出，均须按 `LCP Terminate -> /remote/logout -> 宿主网络回滚 -> 关闭 TUN 设备` 的逆序完成资源清理。
- **非特权可测性**：通过 `WithDevice`、`WithRootCAs` 与 `WithSAMLListenAddr` 支持注入 `fortitest` 内存假网卡与假网关，在无 root 权限下完成完整端到端回归测试。

## 验证

```bash
go test -race ./fortivpn/engine/...
```
