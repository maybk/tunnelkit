# 内核会话编排 (`fortivpn/engine`)

将 HTTPS 控制面认证、PPP 链路协商、跨平台 TUN 虚拟网卡、宿主路由/DNS 配置与双向数据包转发组装为完整的 VPN 会话生命周期。

## 目录结构

- `engine.go`：引擎配置选项、会话运行入口 `Run`、无网卡握手自检 `Check` 与残留网络恢复 `Recover`。
- `recovery.go`：旧版网络恢复数据的内核导入入口，持久化后由 `Recover` 统一回滚。
- `session.go`：单次尝试与自动重连状态机编排、指数退避重试、免重登重连判定及反序优雅清理。
- `link.go`：绑定底层 `sslvpn.TunnelConn` 与 `ppp.Session`，管理控制队列、数据队列及 PPP 协商就绪等待。
- `netcfg.go`：汇总网关下发与用户自定义的路由和 DNS 规则，驱动虚拟网卡打开及宿主网络初次应用或重连差量更新。
- `buffers.go`：PPP/隧道帧的前置空间布局与分级缓冲池，读取缓冲覆盖完整 IPv4 长度。
- `pump.go`：TUN 设备与 TLS 隧道之间的双向并发数据包转发、LCP Echo 死链探测及流量统计采样。

## 开发规范

- **缓冲区契约**：复用前置空间原地封装帧头；读取容量须覆盖完整 IPv4 报文，发送队列保持有界。
- **导入隔离**：仅允许由 `cmd/tunnelkit-helper`、`internal/helper`、`fortivpn/cmd/fortivpn` 及测试加载；桌面 GUI 主进程严禁直接导入本包。
- **退出清理**：关闭链路后，依次恢复宿主网络、关闭 TUN 并尝试注销。网络恢复失败须保留记录并向上报错。
- **非特权可测性**：通过 `WithDevice`、`WithRootCAs` 与 `WithSAMLListenAddr` 支持注入 `fortitest` 内存假网卡与假网关，在无 root 权限下完成完整端到端回归测试。

## 验证

```bash
go test -race ./fortivpn/engine/...
```
