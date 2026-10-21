# PPP 协议栈 (`internal/ppp`)

本包实现跑在 Fortinet `0x5050` 隧道帧之上的精简 PPP 状态机，覆盖链路控制、IPv4 地址协商与心跳保活。

## 职责

- [packet.go](packet.go)：PPP 控制报文（Code / Identifier / Length）与 TLV 选项编解码。
- [fsm.go](fsm.go)：RFC 1661 协商状态机驱动。
- [lcp.go](lcp.go)：LCP（`0xc021`）协商，处理 MRU 与 Magic-Number 并拒绝网关认证重协商。
- [ipcp.go](ipcp.go)：IPCP（`0x8021`）协商，遵循网关 `Configure-Nak` 收敛分配客户端 IPv4、主备 DNS。
- [echo.go](echo.go)：周期发送 `Echo-Request`（Code `9`）并响应对端 `Echo-Reply`（Code `10`），连续超时判定死链。
- [server.go](server.go)：供单元测试与 `fortitest` 假网关复用的服务端 PPP 会话实现。

## 验证

```bash
cd fortivpn && go test -race ./internal/ppp/...
```
