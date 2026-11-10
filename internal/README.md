# 桌面端与辅助进程内部模块 (`internal`)

承载 TunnelKit 桌面客户端宿主服务、特权辅助进程编排、本地 IPC、凭据加密、配置持久化与版本自更新的内部子包。

## 目录结构

- `api/`：暴露给 Wails 的桌面服务薄适配层。
- `app/`：桌面应用依赖装配、窗口、托盘、安全退出与自更新协调。
- `vpn/`：桌面端连接状态机、交互挑战转发、会话接管、维护门禁与网络恢复。
- `profile/`：连接配置增删改查、字段校验、配置文本转换与加密密码存储。
- `helper/`：按单次 VPN 会话运行的特权辅助服务端，驱动 `fortivpn/engine`。
- `helperclient/`：辅助进程按需拉起、客户端证书预读、加密会话票据与 IPC 客户端。
- `privilege/`：三端受保护辅助组件的安装、权限校验、系统授权、服务注册、进程查询与运行路径。
- `ipc/`：跨平台本地 IPC 传输、对端身份校验与长度前缀帧协议。
- `credentials/`：本地主密钥保护与 AES-256-GCM 凭据加解密。
- `storage/`：`~/.tunnelkit` 目录路径管理、原子文件替换与日志尾部脱敏读取。
- `update/`：多源版本检查、SHA-256 校验、三端原地替换回滚与重启交接。

## 开发规范

- **单向依赖与内核隔离**：桌面进程子包仅允许导入 `fortivpn` 根包，严禁导入 `fortivpn/engine`；领域逻辑下沉至对应子包，`internal/api` 只做薄转发。
- **凭据与提权边界**：连接密码、客户端私钥与会话令牌仅在内存或已鉴权 IPC 通道中流转，落盘必须经 `internal/credentials` 加密；辅助组件与原地更新提权分别收敛在 `internal/privilege` 与 `internal/update`。

## 验证

```bash
go test -race ./internal/...
```
