# 非特权集成测试套件 (`fortivpn/fortitest`)

提供进程内内存假 FortiGate 网关与内存虚拟网卡，无需真实 TUN 设备或管理员权限即可验证完整 SSL-VPN 握手、重连与双向数据包转发。

## 目录结构

- `gateway.go`：基于 `httptest.Server` 的内存假 FortiGate 网关，支持密码认证、双因素挑战、SAML 回调、XML 隧道配置下发、`0x5050` 隧道升级与服务端 PPP 回环。
- `memdev.go`：实现 `engine.Device` 接口的内存双向三层报文通道，用于注入宿主机出站包并断言入站回包。

## 开发规范

- **测试专用**：仅通过 `engine.WithDevice`、`engine.WithRootCAs` 或测试构造注入，严禁在生产代码中引用。
- **资源回收**：测试结束时必须调用 `Gateway.Close()` 与 `MemDevice.Close()`，确保释放回环监听端口与后台协程。

## 验证

```bash
go test -race ./fortivpn/...
```
