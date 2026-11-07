# 数据面 PPP 协议栈 (`fortivpn/internal/ppp`)

实现跑在 Fortinet `0x5050` 隧道帧之上的精简 RFC 1661 PPP 状态机，覆盖链路控制（LCP）、IPv4 地址协商（IPCP）与心跳保活（Echo）。

## 目录结构

- `packet.go`：PPP 控制报文与 TLV 选项编解码。
- `fsm.go`：RFC 1661 有限状态机核心生命周期、定时器代次管理与重传计数驱动。
- `fsm_handlers.go`：`Configure-Request`、`Configure-Ack`、`Configure-Nak/Reject`、`Terminate-Request/Ack` 状态转换处理器。
- `lcp.go`：LCP 协议处理器，协商 MRU 与 Magic-Number 并拒绝隧道内二次密码认证请求。
- `ipcp.go`：IPCP 协议处理器，遵循网关 `Configure-Nak` 收敛分配客户端 IPv4 地址与主备 DNS。
- `echo.go`：周期性发送 `Echo-Request` 并校验对端 `Echo-Reply`，连续超时判定死链。
- `server.go`：供单元测试与 `fortitest` 内存假网关复用的服务端 PPP 会话实现。
- `UPSTREAM.md`：PPP 报文编解码与测试服务端部分参考上游开源实现的版权与来源说明。

## 开发规范

- **串行状态机约束**：`Session` 内部状态由调用方或事件循环串行驱动，出站控制帧通过 `Events.Send` 回调异步入队，严禁在状态机回调内阻塞等待网络 I/O。
- **协商上限守卫**：`Configure-Nak/Reject` 与超时重传均受最大次数约束，超限后必须主动迁移至 `StateStopped` 并通知上层终止握手。

## 验证

```bash
go test -race ./fortivpn/internal/ppp/...
```
