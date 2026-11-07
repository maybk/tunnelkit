# 本地 IPC 协议与传输 (`internal/ipc`)

定义桌面主进程与特权辅助进程之间的消息契约、有界帧传输、令牌摘要校验与跨平台对端身份验证。

## 目录结构

- `message.go`：IPC 协议版本号、消息信封类型、严格 JSON 解码器与恒定时间令牌 SHA-256 校验。
- `conn.go`：4 字节大端长度前缀与 JSON 载荷帧读写封装，内置 1 MiB 最大帧限制与写互斥锁。
- `unavailable.go`：区分本地 IPC 端点不存在与超时、鉴权失败等传输错误。
- `transport_darwin.go`：macOS Unix Domain Socket 监听、拨号及基于 `LOCAL_PEERCRED` 的对端 UID 校验。
- `transport_linux.go`：Linux Unix Domain Socket 监听、拨号及基于 `SO_PEERCRED` 的对端 UID 校验。
- `transport_windows.go`：Windows Named Pipe 监听、拨号、DACL 安全描述符及对端 SID 校验。
- `transport_other.go`：非支持平台下的 IPC 传输空实现。

## 开发规范

- **双重鉴权契约**：传输层在接收业务请求前，必须同时验证操作系统内核提供的对端进程身份（UID/SID）与按会话生成的 256-bit 随机令牌。
- **帧大小与版本守卫**：所有消息读写须受 `MaxFrameSize` 上限约束并启用 `DisallowUnknownFields`；修改 `message.go` 契约时必须同步递增 `ProtocolVersion`。

## 验证

```bash
go test -race ./internal/ipc/...
```
