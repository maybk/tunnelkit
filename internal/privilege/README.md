# 特权组件部署 (`internal/privilege`)

本包负责三端特权辅助组件的受保护安装、完整性与权限校验、系统授权调用、服务注册及运行路径管理，供 `helper` 与 `helperclient` 复用，不依赖 VPN、协议内核、`update` 或 Wails。

## 职责

- [paths.go](paths.go) / [paths_unix.go](paths_unix.go) / [paths_windows.go](paths_windows.go)：管理受保护安装路径与按用户隔离的 `run/<uid-or-sid>` 运行目录，校验调用方身份与通信端点归属。
- [install.go](install.go) / [install_unix.go](install_unix.go) / [install_windows.go](install_windows.go)：校验并原子部署受保护组件（SHA-256 摘要、Unix root 所有权与 `setuid` 权限、Windows 所有者与 DACL）。
- [elevation_darwin.go](elevation_darwin.go) / [elevation_linux.go](elevation_linux.go) / [elevation_windows.go](elevation_windows.go)：在首次安装或组件内容升级时拉起系统管理员授权（macOS `osascript`、Linux `pkexec`、Windows `ShellExecuteExW(runas)`）。
- [service_windows.go](service_windows.go)：注册、更新与卸载按需手动启动的 `LocalSystem` 服务 `TunnelKitHelper`，配置仅允许普通交互用户查询与启动的 SDDL。
- [lock.go](lock.go) / [lock_unix.go](lock_unix.go) / [lock_windows.go](lock_windows.go)：跨进程系统级会话文件锁，串行化宿主网络修改与组件升级。

## 验证

```bash
go test -race ./internal/privilege/...
```
