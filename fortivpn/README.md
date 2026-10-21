# fortivpn

`fortivpn` 是独立的纯 Go Fortinet SSL-VPN 协议内核与命令行客户端，零外部业务耦合，不依赖 `tunnelkit/internal/**` 或 Wails。

## 目录与职责

- [config.go](config.go) / [configtext.go](configtext.go)：`Config` 配置模型、校验与 `key = value` 文本双向序列化。
- [event.go](event.go) / [errors.go](errors.go)：会话阶段事件（`Phase`）、交互式挑战请求（`Prompt`）与结构化错误定义。
- [engine/engine.go](engine/engine.go) / [engine/session.go](engine/session.go) / [engine/pump.go](engine/pump.go)：单次连接与自动重连编排、双向数据包转发及流量统计。
- [internal/sslvpn/](internal/sslvpn/README.md)：HTTPS 控制面认证、2FA / SAML、隧道 XML 解析、裸 TLS 连接劫持与 `0x5050` 6 字节帧编解码。
- [internal/ppp/](internal/ppp/README.md)：PPP 数据面协商（LCP、IPCP）与 LCP Echo 保活。
- [internal/netdev/](internal/netdev/README.md)：跨平台虚拟 TUN 网卡（macOS `utun`、Windows Wintun、Linux `/dev/net/tun`）。
- [internal/netcfg/](internal/netcfg/README.md)：宿主路由、分流 DNS 与异常恢复日志（`journal.json`）。
- [fortitest/](fortitest/)：内存假 FortiGate 网关与内存虚拟网卡，支撑无特权端到端集成测试。
- [cmd/fortivpn/main.go](cmd/fortivpn/main.go)：独立命令行客户端入口。

## 会话生命周期

1. **控制面认证**：HTTPS 登录 `/remote/logincheck`（或 SAML 浏览器回调），按需通过 `Handler.Prompt` 完成双因素挑战，获取 `SVPNCOOKIE`。
2. **隧道建链**：请求 `/remote/fortisslvpn_xml` 解析 IP/路由/DNS，随后发送 `GET /remote/sslvpn-tunnel` 劫持底层 TLS 连接切换为 `0x5050` 二进制帧流。
3. **PPP 协商**：在二进制帧流上完成 LCP 与 IPCP 协商，取得网关分配的内网 IPv4 地址与 DNS。
4. **网卡与路由配置**：打开平台虚拟 TUN 网卡，先将回滚计划写入 `journal.json`，再下发网关 `/32` 保护路由、子网路由与 DNS。
5. **转发与保活**：启动 TUN 与 TLS 双向转发协程及周期性 LCP Echo 探测；断开或异常退出时幂等回滚宿主路由与 DNS。

## 验证

```bash
cd fortivpn && go test -race ./...
```
