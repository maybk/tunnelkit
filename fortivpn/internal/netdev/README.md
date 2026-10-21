# 跨平台虚拟网卡 (`internal/netdev`)

本包屏蔽各操作系统三层虚拟网卡（TUN）的系统调用差异，向 `fortivpn/engine` 提供统一的純 IP 报文读写接口。

## 职责

- [device.go](device.go)：虚拟网卡接口抽象与平台地址配置入口。
- [tun_darwin.go](tun_darwin.go)：macOS 基于 `AF_SYSTEM` / `SYSPROTO_CONTROL` 打开内核 `com.apple.net.utun_control`（`utunN`），自动剥离/附加 4 字节 `AF_INET` 协议族头部。
- [tun_windows.go](tun_windows.go)：Windows 动态加载 `wintun.dll` 驱动创建环形缓冲区会话读写三层 IP 包。
- [tun_linux.go](tun_linux.go)：Linux 通过 `/dev/net/tun` 与 `TUNSETIFF`（`IFF_TUN | IFF_NO_PI`）创建虚拟接口。

## 验证

```bash
cd fortivpn && go test -race ./internal/netdev/...
```
