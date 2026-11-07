# Fortinet SSL-VPN 协议内核 (`fortivpn`)

纯 Go 实现的跨平台 Fortinet SSL-VPN 协议内核与命令行客户端，提供公共配置模型、结构化事件与错误契约及会话引擎，零外部业务耦合。

## 目录结构

- `doc.go`：包级说明与架构边界定义。
- `config.go`：内核配置模型、敏感凭据包装类型与合法性校验。
- `configtext.go`：`key = value` 兼容配置文本与结构化 `Config` 的双向解析与序列化。
- `event.go`：会话阶段枚举、隧道信息、流量统计、交互挑战与事件模型。
- `errors.go`：结构化错误码与 `*fortivpn.Error` 包装及分类判断辅助。
- `engine/`：SSL-VPN 会话生命周期编排、自动重连、宿主网络配置与双向数据包转发引擎。
- `fortitest/`：进程内内存假 FortiGate 网关与内存虚拟网卡，支撑无特权端到端集成测试。
- `internal/`：内核私有子包，涵盖控制面、PPP 协议栈、TUN 虚拟网卡与宿主路由/DNS 恢复。
- `cmd/`：独立内核命令行客户端入口。

## 开发规范

- **零外部耦合**：`fortivpn/**` 严禁导入 `tunnelkit/internal/**` 或 Wails，保持可随时独立拆出为单独仓库。
- **宿主调用边界**：根包仅依赖 Go 标准库暴露公共契约；桌面 GUI 主进程仅允许导入根包 `tunnelkit/fortivpn`，严禁直接导入 `tunnelkit/fortivpn/engine`。
- **配置与错误契约**：进入引擎的配置必须通过 `Config.Validate()` 校验；所有对外错误须封装为结构化 `*fortivpn.Error`，不在内核层硬编码界面本地化文案。

## 验证

```bash
go test -race ./fortivpn/...
```
