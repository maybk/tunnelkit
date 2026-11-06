# 特权辅助服务与本地 IPC (`internal/helper`)

桌面主进程以普通用户权限运行，虚拟网卡创建与路由/DNS 修改由按会话生命周期运行的 `tunnelkit-helper` 承担。

## 职责

- [server.go](server.go)：辅助进程监听生命周期、调用方身份与一次性令牌校验、单会话请求（`start` / `recover` / `answer` / `stop`）分发。
- [session.go](session.go)：驱动 `fortivpn/engine` 执行 VPN 会话，桥接异步交互挑战（OTP / FortiToken Push / SAML）并在会话结束后自动退出。
- [logfile.go](logfile.go)：脱敏日志格式化器，自动遮蔽密码、`SVPNCOOKIE`、Token 与验证码并通过已鉴权 IPC 流式回传给桌面进程落盘。
- [cli.go](cli.go) / [privilege_unix.go](privilege_unix.go) / [privilege_windows.go](privilege_windows.go)：受限启动参数解析、Unix `setuid` 身份与环境净化、Windows 单会话服务入口。
- [../privilege/](../privilege/)：受保护组件安装、完整性校验与系统授权；[../helperclient/](../helperclient/) 负责桌面进程侧按需启动与会话控制。
- [../ipc/](../ipc/)：基于 Unix Domain Socket / Windows Named Pipe 的帧协议与 Peer UID/SID 凭据校验。

## 安全与生命周期约束

1. **按会话运行**：首次安装或升级时完成一次系统授权部署，日常连接直接拉起受保护组件；隧道断开即自动清理并退出，不保留常驻后台进程。
2. **双因子本地鉴权**：IPC 连接必须同时通过操作系统调用方 UID 校验（或 Windows Named Pipe SID DACL 约束）与 256-bit 一次性随机令牌校验。
3. **凭据不落盘**：密码、客户端私钥与交互验证码仅通过已鉴权的本地 IPC 内存通道传输，严禁出现在命令行参数或临时文件中。

## 验证

```bash
go test -race ./internal/ipc/... ./internal/helper/... ./internal/helperclient/...
```
