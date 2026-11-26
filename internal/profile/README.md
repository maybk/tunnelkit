# 连接配置与凭据存储 (`internal/profile`)

管理 VPN 连接配置、应用设置的持久化与校验、`key = value` 文本双向转换以及连接密码的加密隔离存储。

## 目录结构

- `profile.go`：连接配置模型、保存入参、对外视图与内核 `fortivpn.Config` 转换。
- `validate.go`：配置字段归一化、IPv4 CIDR 补充路由校验、64 位十六进制证书指纹校验与表单、配置文本双向同步。
- `store.go`：配置与设置存取入口、关联密码维护及启动期权限和旧密钥整理。
- `persistence.go`：配置编解码、UTF-8 BOM 容错、旧格式兼容与私有原子写入。
- `settings.go`：应用设置（主题模式、自动检查更新）的字段合并、归一化与 `settings.json` 原子持久化。
- `secret.go`：连接密码经 `internal/credentials` 加密存取至 `secrets.json`，防止缺密钥混写。

## 开发规范

- **凭据隔离与防混写**：`profiles.json` 不保存密码，已保存密码不回填配置文本，对外通过 `hasPassword` 标识状态；`secrets.json` 已有其他密码且缺失 `secret.key` 时必须阻止写入。
- **格式版本与跨端兼容**：配置与密码文件独立维护格式版本，容错 UTF-8 BOM 并以私有文件权限原子落盘；旧格式在保存时升级，未知版本报错并保留原文件。
- **更新保留原密码**：编辑已有连接配置且密码输入为空时，自动保留 `secrets.json` 中已保存的加密密码；新建密码认证配置时密码必填。

## 验证

```bash
go test -race ./internal/profile/...
```
