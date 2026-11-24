# 本地文件存储基础设施 (`internal/storage`)

统一管理 `~/.tunnelkit` 数据目录路径布局、跨平台原子文件替换与有界日志脱敏读取。

## 目录结构

- `paths.go`：数据根目录、`profiles.json`、`secrets.json`、`secret.key`、会话票据 `run/session.json`、旧版网络恢复记录及连接日志目录的标准路径解析。
- `atomic.go`：在目标同级目录创建临时文件，完成写入与刷盘后原子重命名替换，失败时自动清理残留临时文件。
- `regular.go`：常规文件不跟随打开、符号链接与重解析点拒绝、有界读取及返回前关闭句柄原语。
- `regular_unix.go`：Unix 平台非阻塞且不跟随符号链接的底层打开实现。
- `regular_windows.go`：Windows 不跟随重解析点的底层打开实现。
- `regular_other.go`：其他平台的常规文件打开兜底实现。
- `document.go`：版本化 JSON 封套编解码；版本由调用模块指定，拒绝未知版本与缺失载荷。
- `tail.go`：拒绝符号链接与重解析点、有界读取日志尾部，丢弃缺少上下文的行片段并复用凭据脱敏。

## 开发规范

- **原子落盘与有界读取**：所有持久化配置、加密凭据与会话票据写入必须经由 `atomic.go` 完成；内部受管文件读取须复用 `regular.go` 拒绝符号链接、重解析点与超大文件，严禁无界读取或截断覆写原文件。
- **依赖边界**：本包仅依赖 Go 标准库、系统调用原语与 `internal/redact`，严禁导入上层业务包或 Wails；日志读取的兜底脱敏不替代写入前的实时脱敏。

## 验证

```bash
go test -race ./internal/storage/...
```
