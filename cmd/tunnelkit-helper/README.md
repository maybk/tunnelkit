# 特权辅助程序入口 (`cmd/tunnelkit-helper`)

提供 `tunnelkit-helper` 命令行入口，委托 `internal/helper` 与 `internal/privilege` 处理单会话 IPC 服务与受保护安装。

## 目录结构

- `main.go`：解析版本、安装卸载与 `serve` 子命令，委托下层模块执行并返回退出码。

## 开发规范

- **职责边界**：仅做进程入口装配，严禁导入桌面 GUI 包或 Wails。

## 验证

```bash
go test -race ./cmd/tunnelkit-helper/...
```
