# 跨平台 TUN 虚拟网卡 (`fortivpn/internal/netdev`)

屏蔽各操作系统三层虚拟网卡的系统调用差异，向 `fortivpn/engine` 提供统一的纯 IP 报文读写接口。

## 目录结构

- `device.go`：虚拟网卡 `Device` 接口定义、默认 MTU 归一化与平台打开入口。
- `tun_darwin.go`：macOS 基于 `AF_SYSTEM` 打开内核 `utun` 接口，自动剥离与附加 4 字节 `AF_INET` 协议族头部。
- `tun_windows.go`：Windows 基于 `wireguard/tun` 与 `wintun.dll` 创建环形缓冲区会话读写三层 IP 报文。
- `tun_linux.go`：Linux 通过 `/dev/net/tun` 与 `TUNSETIFF` 创建非持久化三层虚拟接口。
- `tun_other.go`：非支持平台下返回 `CodeUnsupportedPlatform` 的空实现。

## 开发规范

- **纯三层报文契约**：`ReadPacket` 与 `WritePacket` 交互载荷固定为原始 IPv4 报文，平台特有的帧头必须在平台文件内部封装剥离。
- **关闭幂等安全**：`Close()` 必须保证并发与重复调用安全，关闭后及时唤醒阻塞中的读写协程。

## 验证

```bash
go test -race ./fortivpn/internal/netdev/...
```
