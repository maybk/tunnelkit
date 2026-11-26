# 本地文件存储基础设施 (`internal/storage`)

统一管理 `~/.tunnelkit` 数据目录路径布局、跨平台原子文件替换、私有文件权限收紧与有界日志脱敏读取。

## 目录结构

- `paths.go`：数据根目录、`profiles.json`、`settings.json`、`secrets.json`、`secret.key`、会话票据 `run/session.json`、旧版网络恢复记录及连接日志目录的标准路径解析。
- `atomic.go`：同目录临时文件原子替换、重命名前置钩子、私有文件落盘与 Windows 只读目标替换重试。
- `acl_windows.go`：Windows 句柄级受保护 DACL 收紧、宽权限检测与常规文件只读位清理（不跟随重解析点）。
- `acl_other.go`：类 Unix 句柄级 `0600` 权限收紧与宽权限检测（不跟随符号链接）。
- `regular.go`：常规文件不跟随打开、符号链接与重解析点拒绝、有界读取及返回前关闭句柄原语。
- `regular_unix.go`：Unix 平台非阻塞且不跟随符号链接的底层打开实现。
- `regular_windows.go`：Windows 不跟随重解析点的底层打开实现。
- `regular_other.go`：其他平台的常规文件打开兜底实现。
- `document.go`：UTF-8 BOM 剥离与版本化 JSON 封套编解码；版本由调用模块指定，拒绝未知版本与缺失载荷。
- `tail.go`：拒绝符号链接与重解析点、有界读取日志尾部，丢弃缺少上下文的行片段并复用凭据脱敏。

## 开发规范

- **原子落盘与私有权限**：配置、加密凭据与会话票据写入须经 `atomic.go`，私有文件在替换前收紧权限（Unix `0600`、Windows 受保护 DACL）；内部读取须复用 `regular.go` 拒绝符号链接与重解析点，严禁无界读取或截断覆写。
- **依赖边界**：本包仅依赖 Go 标准库、系统调用原语与 `internal/redact`，严禁导入上层业务包或 Wails；日志读取的兜底脱敏不替代写入前的实时脱敏。

## 验证

```bash
go test -race ./internal/storage/...
```
