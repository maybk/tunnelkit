# 特权组件安装与系统授权 (`internal/privilege`)

负责三端特权辅助组件的受保护安装、完整性与权限校验、系统授权调用、服务注册、进程查询及运行路径管理。

## 目录结构

- `paths.go`：受保护的用户运行期会话目录与网络恢复日志路径解析。
- `paths_unix.go`：macOS / Linux 受保护安装路径、状态根目录及 Unix Domain Socket 端点归属校验。
- `paths_windows.go`：Windows 受保护安装路径、`ProgramData` 状态目录、调用方 SID 及 Named Pipe 端点格式校验。
- `paths_unsupported.go`：其他平台路径与身份校验不支持入口。
- `acl_windows.go`：Windows 安全描述符、重解析点校验与访问控制列表设置。
- `install.go`：受保护组件按需安装与升级编排、SHA-256 完整性比对及同目录原子替换。
- `install_unix.go`：macOS / Linux 父目录链权限校验、受信任系统可执行文件校验、root 所有权与 `setuid` 部署及卸载。
- `install_windows.go`：Windows 受保护目录层级建立、组件状态检查与部署。
- `install_unsupported.go`：其他平台安装与卸载不支持入口。
- `elevation_darwin.go`：macOS 通过 `/usr/bin/osascript` 请求系统管理员授权执行安装。
- `elevation_linux.go`：Linux 校验受信任系统路径后，通过 PolicyKit `pkexec` 或已授权 `sudo -n` 请求管理员权限。
- `elevation_windows.go`：Windows 通过 `ShellExecuteExW` `runas` 触发 UAC 提权执行服务安装。
- `elevation_unsupported.go`：其他平台提权安装不支持入口。
- `service_windows.go`：Windows 按需启动 `LocalSystem` 服务 `TunnelKitHelper` 的注册、SDDL 配置、状态校验与卸载。
- `service_other.go`：非 Windows 平台服务状态默认占位实现。
- `lock.go`：系统级会话排他锁入口，串行化宿主网络变更与组件升级。
- `lock_unix.go`：Unix 平台基于 `flock` 的非阻塞排他文件锁实现。
- `lock_windows.go`：Windows 基于 `LockFileEx` 的非阻塞排他文件锁实现。
- `lock_unsupported.go`：其他平台文件锁不支持入口。
- `process.go`：三端共用的进程存活查询入口与 PID 校验。
- `process_unix.go`：Unix 平台基于 `kill(pid, 0)` 查询进程，权限不足仍视为存在。
- `process_windows.go`：Windows 进程句柄查询，拒绝访问时回退进程快照。
- `process_unsupported.go`：其他平台进程查询不支持入口。
- `quote_sh.go`：`sh` 参数转义与命令行拼装。

## 开发规范

- **零上层耦合**：不引用任何 `tunnelkit/*` 子包或 Wails。
- **防链接与防篡改**：受保护目录及祖先链拒绝符号链接、重解析点与普通用户写权限；仅在首次安装、组件更新或可修复权限漂移时请求管理员授权。

## 验证

```bash
go test -race ./internal/privilege/...
```
