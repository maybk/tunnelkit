# 本地凭据加密 (`internal/credentials`)

提供基于 AES-256-GCM 与平台密钥保护的可复用 `Sealer`，供连接密码与辅助进程会话票据加密落盘。

## 目录结构

- `sealer.go`：生成并加载 `~/.tunnelkit/secret.key` 本地主密钥，使用 AES-256-GCM 与随机 Nonce 加解密数据。
- `key_protection_windows.go`：Windows 平台通过当前用户 DPAPI 封装落盘主密钥。
- `key_protection_other.go`：macOS 与 Linux 平台通过 `0600` 文件权限保护落盘主密钥。

## 开发规范

- **失败显式报错**：主密钥损坏或密文篡改导致认证失败时必须直接返回错误，严禁静默重新生成主密钥覆盖已有密文。
- **保密边界**：严禁将主密钥字节、明文凭据或密文输出到日志或错误消息中。

## 验证

```bash
go test -race ./internal/credentials/...
```
