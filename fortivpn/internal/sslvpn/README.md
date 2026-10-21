# SSL-VPN 控制面与帧协议 (`internal/sslvpn`)

本包实现 Fortinet SSL-VPN 的 HTTPS 控制面认证、隧道参数获取、底层连接劫持及 6 字节二进制帧编解码。

## 职责

- [client.go](client.go) / [trust.go](trust.go)：TLS 客户端构建、系统根证书校验与服务端叶子证书 SHA-256 指纹固定（Pinning）。
- [auth.go](auth.go)：`/remote/login` 预检、`/remote/logincheck` 表单认证、双因素挑战（OTP / FortiToken Push `ret=2`）与 `/remote/logout` 注销。
- [saml.go](saml.go)：本地回环 HTTP 监听接收 SAML 浏览器单点登录回调并提取 `SVPNCOOKIE`。
- [tunconf.go](tunconf.go)：请求并解析 `/remote/fortisslvpn_xml`（含 IPv4 地址、MTU、拆分路由 `split-tunnel-info` 与 DNS）。
- [tunnel.go](tunnel.go)：发送 `GET /remote/sslvpn-tunnel` 伪装升级请求并劫持底层 `tls.Conn`。
- [frame.go](frame.go)：Fortinet SSL-VPN 6 字节帧头读写器与粘包/半包重组。

## 帧格式

隧道建立后，所有 PPP 控制帧与 IPv4 数据包均封装在 6 字节大端序帧头中传输：

```text
+-------------------+-------------------+-------------------+-------------------+
| Total Length (2B) | Magic 0x5050 (2B) |  PPP Proto (2B)   |   Payload (N B)   |
+-------------------+-------------------+-------------------+-------------------+
```

- `Total Length`：包含 6 字节头部在内的整帧字节长度（`6 + len(Payload)`）。
- `Magic`：固定常数 `0x5050`。
- `PPP Proto`：`0xc021`（LCP）、`0x8021`（IPCP）或 `0x0021`（IPv4 数据包）。

## 验证

```bash
cd fortivpn && go test -race ./internal/sslvpn/...
```
