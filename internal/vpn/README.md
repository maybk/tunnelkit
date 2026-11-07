# 桌面连接状态机 (`internal/vpn`)

本包负责桌面端 VPN 连接生命周期编排、交互式挑战转发、运行期会话接管与异常网络恢复。

## 职责

- [service.go](service.go) / [state.go](state.go) / [log.go](log.go) / [maintenance.go](maintenance.go)：连接状态机（`disconnected`、`connecting`、`connected`、`disconnecting`）、串行事件广播、运行日志读写与自更新维护门禁。
- [connect.go](connect.go) / [disconnect.go](disconnect.go) / [shutdown.go](shutdown.go)：连接启动、切换确认、握手期取消、交互挑战应答（`Answer` / `Cancel`）、平滑断开与应用关闭收敛。
- [events.go](events.go)：消费 `tunnelkit-helper` 事件流与脱敏日志，映射为前端可消费的状态快照并以普通用户权限落盘日志。
- [recover.go](recover.go) / [network_recovery.go](network_recovery.go)：通过 `helperclient` 读取 `run/session.json` 加密会话票据；桌面端重启时自动接管存活的辅助进程，或在会话失效、网络清理未完成时触发仅恢复网络的辅助会话回滚残留路由与 DNS。
- [errors.go](errors.go)：将内核错误码映射为用户可读的诊断提示与证书指纹信任引导。

## 验证

```bash
go test -race ./internal/vpn/...
```
