# 特权辅助服务与本地 IPC (`internal/helper`)

桌面主进程以普通用户权限运行，虚拟网卡创建与路由/DNS 修改由按会话生命周期运行的 `tunnelkit-helper` 承担。

## 职责

- [server.go](server.go)：辅助进程监听生命周期、对端身份与一次性令牌校验、单会话请求分发。
- [session.go](session.go)：驱动 `fortivpn/engine` 执行 VPN 会话，桥接异步交互挑战（OTP / FortiToken Push / SAML）并在结束后自动退出。
- [logfile.go](logfile.go)：脱敏日志写入器，落盘前自动遮蔽密码、`SVPNCOOKIE`、Token 与验证码。
- [../helperclient/](../helperclient/)：按需调用系统原生提权（macOS `osascript`、Windows `runas`、Linux `pkexec`）拉起或重连辅助进程。
- [../ipc/](../ipc/)：基于 Unix Domain Socket / Windows Named Pipe 的帧协议与 Peer UID/SID 凭据校验。

## 安全与生命周期约束

1. **按会话提权**：仅在发起 VPN 连接时拉起 `tunnelkit-helper`，隧道断开即自动清理并退出，不安装常驻 root 守护服务。
2. **双因子本地鉴权**：IPC 连接必须同时通过操作系统内核级调用方 UID/SID 校验与 256-bit 一次性随机令牌校验。
3. **凭据不落盘**：密码与交互验证码仅通过已鉴权的本地 IPC 内存通道传输，严禁出现在命令行参数或临时文件中。

## 验证

```bash
go test -race ./internal/ipc/... ./internal/helper/... ./internal/helperclient/...
```
