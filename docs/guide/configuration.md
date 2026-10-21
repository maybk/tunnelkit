# 配置说明

## 本地数据目录

TunnelKit 将所有运行数据保存在本机用户配置目录下（macOS / Linux 为 `~/.tunnelkit/`，Windows 为 `%APPDATA%\TunnelKit\`）：

```text
~/.tunnelkit/
├── profiles.json              # 结构化连接配置，不含密码
├── secrets.json               # 加密保存的连接密码
├── secret.key                 # 本机随机生成的主密钥
├── run/
│   ├── session.json           # 当前活动会话的连接凭据
│   ├── helper.sock            # macOS / Linux 下后台辅助进程监听的本地套接字
│   └── netcfg-journal.json    # 路由与 DNS 变更记录，用于异常退出后自动恢复网络
└── logs/
    └── <profile-id>.log       # 各连接的脱敏运行日志，删除配置时一并清理
```

所有持久化文件均以原子方式写入，避免异常退出导致配置损坏。

## 连接配置字段

新建或编辑连接时，可通过「表单填写」或「配置文本」两种模式维护配置，两边实时双向同步：

| 字段 | 对应配置文本键 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 连接名称 | — | 必填 | 去首尾空白后不能为空 |
| 网关地址 | `host` | 必填 | 域名或 IP 地址，支持在文本模式中写为 `host:port` |
| 端口 | `port` | `443` | 整数，范围 `1`–`65535` |
| 认证方式 | `auth` / `saml-login` | `password` | `password`（账号密码 + 动态口令/推送确认）或 `saml`（浏览器单点登录） |
| 用户名 | `username` | 密码模式必填 | 去首尾空白后不能为空 |
| 密码 | `password` | 新建密码模式必填 | 编辑已有连接时留空表示保持原密码 |
| 自动重连 | `reconnect` / `persistent` | `true` | 网络波动或休眠唤醒后自动重连 |
| 路由策略 | `routes` / `set-routes` | `gateway` | `gateway`（按网关规则转发）、`full`（所有流量走 VPN）、`none`（仅建立隧道） |
| 补充内网网段 | `extra-route` | 空 | 可选；附加 IPv4 CIDR 列表（不可包含 `0.0.0.0/0`） |
| DNS 解析策略 | `dns` / `set-dns` | `split` | `split`（仅内网域名使用 VPN DNS）、`full`（所有域名使用 VPN DNS）、`off`（保持系统原 DNS） |
| 内网域名后缀 | `dns-domain` | 空 | 可选；分流 DNS 额外匹配的域名后缀列表 |
| Realm 认证域 | `realm` | 空 | 可选；网关要求指定虚拟域或专属认证路径时填写 |
| 受信任网关证书指纹 | `trusted-cert` | 空 | 可选；64 位十六进制 SHA-256 服务端证书指纹 |
| 客户端证书 / 私钥 | `user-cert` / `user-key` | 空 | 可选；双向 TLS 认证所需的客户端 PEM 证书与私钥路径 |
| 兼容旧版 VPN 网关 | `legacy-tls` | `false` | 可选；仅在连接旧版 FortiGate 报协议握手失败时开启 |

## 安全与恢复机制

1. **密码加密隔离**：`profiles.json` 不保存密码，前端仅通过布尔字段 `hasPassword` 获知是否已保存密码；密码加密后存入 `secrets.json`。
2. **本机通道传递凭据**：建立连接时，密码与一次性令牌仅通过受保护的本机 IPC 通道传给 `tunnelkit-helper`，不在磁盘上生成含明文密码的临时文件。
3. **日志实时脱敏**：运行日志在写入文件或推送到界面前自动遮蔽密码、验证码与会话凭据。
4. **网络异常自动恢复**：修改系统路由与 DNS 前先记录 `run/netcfg-journal.json`；若进程意外中断，下次启动或连接时会自动清理残留路由与 DNS 配置。
