# 本地凭据加密 (`internal/credentials`)

提供基于跨平台 256 位主密钥、系统私有文件权限与 AES-256-GCM 的可复用 `Sealer`，供连接密码与会话票据加密落盘。

## 目录结构

- `sealer.go`：跨平台主密钥原子生成、BOM/CRLF 容错读取、旧密钥迁移协调、内存归零与 AES-256-GCM 加解密。
- `legacy_windows.go`：Windows 旧版 DPAPI 主密钥的向后兼容解封。
- `legacy_other.go`：非 Windows 平台识别未迁移的旧版 Windows DPAPI 主密钥。

## 开发规范

- **三端通用与平滑兼容**：三端统一以标准 Base64 保存 32 字节随机主密钥，由私有文件权限保护（Unix `0600`、Windows 受保护 DACL）；Windows 旧版 DPAPI 主密钥自动无感迁移，历史密文无需重加密。
- **零脏写与防混写**：只读解密路径绝不新建或删除 `secret.key`；当凭据库已有其他条目时，缺失主密钥必须显式报错阻止混写。
- **保密与依赖边界**：用后立即归零主密钥与明文缓冲，严禁将密钥、明文或密文写入日志与错误消息；本包仅依赖标准库、系统调用原语与 `internal/storage`。

## 验证

```bash
go test -race ./internal/credentials/...
```
