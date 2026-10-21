# 桌面连接状态机 (`internal/vpn`)

本包负责桌面端 VPN 连接生命周期编排、交互式挑战转发、运行期会话接管与异常网络恢复。

## 职责

- [service.go](service.go) / [state.go](state.go)：连接状态机（`Disconnected`、`Connecting`、`Challenging`、`Connected`、`Disconnecting`、`Error`）与串行事件广播。
- [connect.go](connect.go) / [disconnect.go](disconnect.go)：连接启动、切换确认、握手期取消、交互挑战应答（`SubmitChallenge` / `CancelChallenge`）与平滑断开。
- [events.go](events.go)：消费 `tunnelkit-helper` 事件流并映射为前端可消费的状态快照。
- [ticket.go](ticket.go) / [recover.go](recover.go)：管理 `run/active-helper.json` 会话票据；桌面端重启时自动重新接管存活的辅助进程，或在辅助进程已退出且残留 `run/netcfg-journal.json` 时触发提权回滚。
- [errors.go](errors.go)：将内核错误码映射为用户可读的诊断提示与证书指纹信任引导。

## 验证

```bash
go test -race ./internal/vpn/...
```
