# 特权辅助服务 (`internal/helper`)

承载按单次 VPN 会话生命周期运行的 `tunnelkit-helper` 特权服务，监听本机 IPC 通道并驱动 `fortivpn/engine` 与宿主网络恢复。

## 目录结构

- `server.go`：IPC 监听、对端身份（UID/SID）与一次性令牌摘要联合鉴权，分发 `start`、`recover`、`answer` 与 `stop` 指令。
- `session.go`：会话快照维护与交互挑战管理。
- `event_writer.go`：异步发送事件与脱敏日志，优先保留状态和挑战；关键消息溢出时断开连接。
- `logfile.go`：辅助进程结构化日志处理器与密码、Cookie、令牌、验证码实时脱敏格式化器。
- `cli.go`：解析并校验 `serve` 启动参数。
- `privilege_unix.go`：macOS 与 Linux 启动时将 `setuid-root` 进程的真实 UID/GID 同步至 `0` 并净化环境变量与工作目录。
- `privilege_windows.go`：Windows 检测并接入服务控制管理器（SCM），以按需启动的 `TunnelKitHelper` 服务模式运行单次会话。
- `privilege_other.go`：非支持平台下的权限初始化与服务入口空实现。

## 开发规范

- **按会话运行与双重鉴权**：仅在发起 VPN 连接或执行网络恢复时拉起，会话结束即释放跨进程锁并退出；每个 IPC 连接必须同时通过对端系统身份（UID/SID）与 256-bit 随机令牌校验。
- **凭据不落盘与日志脱敏**：密码与私钥仅从鉴权后的 `start` 消息读取，严禁通过命令行参数传递；所有运行日志必须经 `logfile.go` 脱敏后再通过 IPC 回传给桌面进程落盘。

## 验证

```bash
go test -race ./internal/helper/...
```
