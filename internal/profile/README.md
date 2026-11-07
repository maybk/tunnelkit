# 连接配置与凭据存储 (`internal/profile`)

管理 VPN 连接配置的增删改查、字段校验、`key = value` 文本双向转换以及连接密码的加密隔离存储。

## 目录结构

- `profile.go`：连接配置模型、保存入参、对外视图与内核 `fortivpn.Config` 转换。
- `validate.go`：配置字段归一化、IPv4 CIDR 补充路由校验、64 位十六进制证书指纹校验与表单、配置文本双向同步。
- `store.go`：`profiles.json` 列表读写、新建/更新/删除持久化与关联密码协同维护。
- `secret.go`：连接密码经 `internal/credentials` 加密后独立存取至 `secrets.json`。

## 开发规范

- **凭据隔离**：`profiles.json` 与转换出的配置文本严禁包含密码明文；对外视图仅通过 `hasPassword` 标识是否已保存密码。
- **更新保留原密码**：编辑已有连接配置且密码输入为空时，自动保留 `secrets.json` 中已保存的加密密码；新建密码认证配置时密码必填。

## 验证

```bash
go test -race ./internal/profile/...
```
