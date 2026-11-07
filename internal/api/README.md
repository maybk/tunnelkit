# 桌面服务入口 (`internal/api`)

向 Wails 暴露桌面服务接口，仅做参数转发与跨包编排，不承载领域逻辑或 I/O 实现。

## 目录结构

- `vpn.go`：VPN 连接启停、交互挑战应答与取消、状态查询及连接日志读写接口。
- `profile.go`：连接配置增删改查与表单、配置文本双向转换接口。
- `system.go`：应用元信息、版本更新检查、应用内一键更新安装与官方链接打开接口。

## 开发规范

- **职责边界**：通过注入的 `vpn`、`profile`、`update` 子包完成业务操作，严禁直接控制虚拟网卡、修改宿主网络或导入 `fortivpn/engine`。
- **契约同步**：导出方法或 DTO 结构变更后，须执行 `wails3 generate bindings -ts` 更新前端 TypeScript 绑定。

## 验证

```bash
go test -race ./internal/api/...
```
