# 特权辅助进程客户端 (`internal/helperclient`)

负责桌面主进程按需拉起或接入 `tunnelkit-helper`、预读客户端证书、持久化加密会话票据并封装本地 IPC 控制与事件流。

## 目录结构

- `client.go`：IPC 请求关联、控制调用与事件分发。
- `handshake.go`：版本协商、握手超时与取消收尾。
- `legacy_journal.go`：以桌面权限有界读取旧版网络恢复数据，确认恢复后删除旧记录。
- `ticket.go`：加密会话票据的读写与清理。
- `ticket_migrate.go`：旧票据识别与格式转换。
- `launch.go`：辅助程序二进制定位、受保护安装确保、启动参数拼装与就绪握手等待。
- `launch_unix.go`：macOS 与 Linux 直接拉起已受保护安装的 `setuid-root` 辅助进程并监听退出状态。
- `launch_windows.go`：Windows 通过服务控制管理器（SCM）按需启动 `TunnelKitHelper` 服务并传入会话参数。
- `launch_other.go`：非支持平台下的辅助进程启动空实现。
- `process.go`：等待辅助进程真实退出的轮询与通道同步入口。
- `process_unix.go`：Unix 平台基于 `kill(pid, 0)` 的辅助进程存活探测。
- `process_windows.go`：Windows 平台基于 Toolhelp 进程快照的辅助进程存活探测。
- `process_other.go`：非支持平台下的辅助进程存活探测空实现。
- `certificate.go`：在桌面进程内以当前用户权限读取并校验客户端证书与私钥，转换为内存 `Secret` 后通过 IPC 下发。
- `certificate_unix.go`：Unix 平台以非阻塞方式打开用户文件，交由调用方检查普通文件类型与大小。
- `certificate_windows.go`：Windows 平台以当前桌面用户权限打开文件，交由调用方检查类型与大小。
- `certificate_other.go`：非支持平台下的客户端证书文件打开实现。

## 开发规范

- **凭据零暴露启动**：启动参数仅允许传递通信端点、调用方身份标识与令牌 SHA-256 摘要；明文令牌、连接密码与客户端私钥必须通过建立后的鉴权 IPC 通道传输。
- **用户文件不越权打开**：客户端证书与私钥必须由桌面进程以普通用户权限读取为内存 PEM，特权辅助进程不直接打开用户指定路径。

## 验证

```bash
go test -race ./internal/helperclient/...
```
