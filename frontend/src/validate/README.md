# 数据校验 (`frontend/src/validate`)

集中处理表单草稿、OTP、本地缓存与外链校验；结构化输入使用 Valibot，外链按 URL 安全规则校验。

## 目录结构

- `primitives.ts`：网关主机名、端口号、64 位十六进制证书指纹与 CIDR 列表基础 Schema。
- `profile.ts`：VPN 节点配置表单 Schema 与字段级错误映射。
- `vpn.ts`：二次认证 OTP 验证码 Schema 校验。
- `update.ts`：更新发布页 HTTPS 协议与白名单域名校验。
- `storage.ts`：`localStorage` 外观配置 JSON 安全解析与回退。

## 开发规范

- **纯校验层**：仅依赖 `valibot`、`types/`、`constants/`、`utils/` 与 `domain/`，不触发副作用。
