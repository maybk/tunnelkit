# 版本检查与应用内更新 (`internal/update`)

负责多源版本更新检查、发布资产匹配、SHA-256 完整性校验、三端原地替换回滚与父子进程重启交接。

## 目录结构

- `checker.go`：多源 Release 元数据拉取、降级回退、当前平台资产匹配、原地更新能力判定与官方发布地址白名单校验。
- `version.go`：语义化版本号解析、归一化与新旧版本比较。
- `updater.go`：更新包下载、SHA-256 摘要校验、安装前置与结束维护钩子协调及状态流转。
- `archive.go`：更新归档安全解压、路径穿越防护、目标可执行组件提取与备份回滚原语。
- `script_sh.go`：生成 `sh` 多组件替换与回滚脚本。
- `script_cmd.go`：生成 `cmd.exe` 多组件替换与回滚脚本。
- `apply_darwin.go`：macOS `.dmg` 挂载校验、`.app` 应用包备份替换、失败回滚、辅助组件同步更新与 `osascript` 提权回退。
- `apply_linux.go`：Linux 客户端与 `tunnelkit-helper` 二进制备份、原子替换、回滚与 `pkexec` 提权安装。
- `apply_windows.go`：Windows 客户端与辅助进程分阶段重命名替换、失败回滚及 `TunnelKitHelper` 服务同步刷新。
- `apply_other.go`：不支持平台的更新报错实现。
- `apply_elevated_windows.go`：Windows 安装目录受限时的 UAC 提权更新脚本执行。
- `restart_parent.go`：更新重启参数解析，通过公共进程查询等待旧父进程退出。

## 开发规范

- **先校验后停机**：必须先完成更新包下载与 SHA-256 严格校验，再调用前置维护钩子停止活动 VPN 会话；严禁加载或打开非官方白名单域名的发布地址。
- **事务回滚与重启时序**：多组件替换任一步骤失败必须逆序还原 `.old` / `.bak` 备份，回滚仍失败则返回 `ErrRollbackFailed` 并保留备份；`WaitForRestartParent` 必须在主程序最外层优先调用。

## 验证

```bash
go test -race ./internal/update/...
```
