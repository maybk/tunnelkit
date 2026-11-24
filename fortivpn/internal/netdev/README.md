# 跨平台 TUN 虚拟网卡 (`fortivpn/internal/netdev`)

屏蔽各操作系统三层虚拟网卡的系统调用差异，向 `fortivpn/engine` 提供统一的纯 IP 报文单包与批量读写接口。

## 目录结构

- `device.go`：虚拟网卡单包与批量读写契约、16 字节前置空间封装、错误分类与 WireGuard TUN 适配。
- `tun_darwin.go`：macOS `utun` 创建与内核接收缓冲调优。
- `tun_linux.go`：Linux `/dev/net/tun` 虚拟接口创建。
- `tun_windows.go`：Windows Wintun 虚拟适配器创建。
- `tun_unsupported.go`：不支持平台入口。
- `packet_err_unix.go`：Unix 平台单包拒绝与瞬时背压错误分类。
- `packet_err_windows.go`：Windows 平台单包拒绝与瞬时背压错误分类。
- `packet_err_other.go`：其他平台写错分类默认占位实现。

## 开发规范

- **纯三层报文契约**：报文读写交互载荷固定为原始 IPv4 报文，平台特有的帧头必须在平台文件内部封装剥离。
- **错误区分**：写入须区分单报文拒绝或瞬时背压与底层设备致命故障，仅设备级故障触发链路终止。
- **关闭幂等安全**：设备关闭必须保证并发与重复调用安全，关闭后及时唤醒阻塞中的读写协程并等待事件协程退出。

## 验证

```bash
go test -race ./fortivpn/internal/netdev/...
```
