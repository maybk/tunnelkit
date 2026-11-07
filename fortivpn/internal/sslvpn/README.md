# SSL-VPN 控制面与隧道帧协议 (`fortivpn/internal/sslvpn`)

实现 Fortinet SSL-VPN 的 HTTPS 控制面认证、隧道参数获取、底层连接劫持及 `0x5050` 6 字节二进制帧编解码。

## 目录结构

- `client.go`：HTTPS 控制面客户端、网关可达性 TLS 预检、已解析网关 IP 固定与 `/remote/sslvpn-tunnel` 隧道连接建立。
- `trust.go`：TLS 配置构建、系统根证书池加载、双向 TLS 客户端证书装载与服务端叶子证书 SHA-256 指纹固定。
- `auth.go`：`/remote/logincheck` 表单编码与解析、双因素挑战交互及 `/remote/logout` 注销。
- `saml.go`：本地回环 HTTP 监听接收 SAML 浏览器单点登录回调并提取 `SVPNCOOKIE`。
- `tunconf.go`：请求并解析 `/remote/fortisslvpn_xml` 及旧版 `/remote/fortisslvpn` 回退，提取分配 IP、DNS、分流域名与拆分路由并对敏感字段脱敏。
- `tunnel.go`：构造 `GET /remote/sslvpn-tunnel` 升级请求并封装底层 `net.Conn` 为带帧读写锁的 `TunnelConn`。
- `frame.go`：Fortinet SSL-VPN 6 字节大端帧头读写器与粘包、半包重组。
- `UPSTREAM.md`：协议帧与认证表单部分参考上游开源实现的版权与来源说明。

## 开发规范

- **重定向与凭据安全**：HTTP 客户端禁止跨主机跟随重定向；`SVPNCOOKIE`、密码与挑战验证码严禁写入错误描述或未脱敏的 XML 转储。
- **网关 IP 一致性**：完成控制面预检或登录后需固定网关解析出的 IPv4 地址，供后续隧道握手与宿主 `/32` 保护路由复用；连接失败重连时自动重新解析 DNS。

## 验证

```bash
go test -race ./fortivpn/internal/sslvpn/...
```
