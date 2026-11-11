# 配置说明

## 本地数据目录

TunnelKit 将连接配置、加密凭据与运行日志保存在 `~/.tunnelkit/`：

```text
~/.tunnelkit/
├── profiles.json              # 连接配置列表（不含密码）
├── secrets.json               # 加密保存的连接密码
├── secret.key                 # 本机主密钥
├── run/
│   └── session.json           # 当前会话元数据与加密重连令牌
└── logs/
    └── <profile-id>.log       # 各连接的脱敏运行日志（删除配置时一并清理）
```

所有配置文件均以原子方式写入，避免异常中断造成文件损坏，并自动兼容升级旧版本本地数据。

## 连接配置字段

新建或编辑连接时，可通过「表单填写」或「配置文本」维护配置，两种模式实时双向同步：

| 字段 | 对应配置文本键 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 连接名称 | — | 必填 | 去首尾空白后不能为空 |
| 网关地址 | `host` | 必填 | 域名或 IP 地址，文本模式支持 `host:port` |
| 端口 | `port` | `443` | 整数，范围 `1`–`65535` |
| 认证方式 | `auth` / `saml-login` | `password` | `password`（账号密码 / OTP / 推送确认）或 `saml`（浏览器单点登录） |
| 用户名 | `username` | 密码模式必填 | 去首尾空白后不能为空 |
| 密码 | `password` | 新建密码模式必填 | 编辑已有连接时留空表示保留原密码 |
| 自动重连 | `reconnect` / `persistent` | `true` | 网络波动或休眠唤醒后自动恢复隧道 |
| 路由策略 | `routes` / `set-routes` | `gateway` | `gateway`（按网关规则转发）、`full`（全局流量走 VPN）、`none`（不改路由） |
| 补充内网网段 | `extra-route` | 空 | 可选；附加 IPv4 CIDR 列表（不含 `0.0.0.0/0`） |
| DNS 解析策略 | `dns` / `set-dns` | `split` | `split`（内网域名走 VPN DNS）、`full`（全局走 VPN DNS）、`off`（保持原 DNS） |
| 内网域名后缀 | `dns-domain` | 空 | 可选；分流 DNS 额外匹配的域名后缀列表 |
| Realm 认证域 | `realm` | 空 | 可选；网关要求指定认证域时填写 |
| 受信任网关证书指纹 | `trusted-cert` | 空 | 可选；64 位十六进制 SHA-256 服务端证书指纹 |
| 客户端证书 / 私钥 | `user-cert` / `user-key` | 空 | 可选；双向 TLS 认证所需的 PEM 证书与私钥路径 |
| 兼容旧版 VPN 网关 | `legacy-tls` | `false` | 可选；仅在连接旧版 FortiGate 握手失败时开启 |

## 安全与恢复机制

1. **凭据加密隔离**：`profiles.json` 不含密码，前端仅通过 `hasPassword` 获知密码状态；连接密码与 `run/session.json` 会话令牌均由本机密钥加密存储。
2. **本机 IPC 传输**：连接密码、客户端私钥与一次性令牌仅经受保护的本机 IPC 通道传给 `tunnelkit-helper`，不落盘为明文临时文件或命令行参数。
3. **日志与导出脱敏**：运行日志在发送与落盘前实时脱敏，读取与复制时二次兜底；网关 XML 转储自动遮蔽敏感字段，仅保留网络诊断信息。
4. **按需运行与网络恢复**：辅助组件首次安装或升级时部署至系统保护目录，按单次会话运行且断开即退出；修改路由与 DNS 前先写入受保护目录的 `run/<uid-or-sid>/netcfg-journal.json`，异常中断后自动按逆序幂等恢复宿主网络。
