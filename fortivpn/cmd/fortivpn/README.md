# 独立内核命令行工具 (`fortivpn/cmd/fortivpn`)

独立于桌面 GUI 的纯 Go Fortinet SSL-VPN 命令行入口，用于直接建立隧道、无网卡握手自检与残留网络恢复。

## 目录结构

- `main.go`：解析 `connect`、`check`、`recover` 子命令，驱动 `fortivpn/engine` 执行并处理交互与信号退出。

## 开发规范

- **零桌面耦合**：仅依赖 `fortivpn` 与 `fortivpn/engine`，严禁导入 `tunnelkit/internal/**` 或 Wails。

## 验证

```bash
go build -o /dev/null ./fortivpn/cmd/fortivpn
```
