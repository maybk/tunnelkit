# 项目概览与目录结构

TunnelKit 是基于 Wails v3、Go 与 Vue 3 + TypeScript 构建的跨平台（macOS / Windows / Linux）Fortinet SSL-VPN 桌面与菜单栏客户端。

## 架构

```text
Vue 前端 -> Wails bindings -> internal/api -> internal/vpn -> internal/helperclient -> tunnelkit-helper -> fortivpn
```

| 模块 | 职责 | 边界约束 |
| --- | --- | --- |
| [`frontend`](../../frontend/README.md) | 桌面工作台界面、表单与配置文本双向编辑、认证弹窗、状态与日志展示 | 不直接读写本地文件、不保存密码明文 |
| [`internal/app`](../../internal/app/README.md) | 应用启动装配、桌面标题栏配置、菜单栏常驻、自更新协调与事件桥接 | 不承载连接状态机或配置校验逻辑 |
| [`internal/api`](../../internal/api/README.md) | 暴露给前端的 Wails 服务薄适配层 | 仅做参数转发、配置文本转换与状态拼装 |
| [`internal/vpn`](../../internal/vpn/README.md) | 桌面端连接状态机、交互挑战桥接、连接切换、维护门禁与网络恢复 | 通过 `helperclient` 控制辅助进程，不导入 `fortivpn/engine` |
| [`internal/helper`](../../internal/helper/README.md) | 单会话特权辅助服务、异步事件流推送、脱敏日志与网络恢复 | 仅响应通过本机身份与一次性令牌校验的连接 |
| [`internal/helperclient`](../../internal/helperclient/README.md) | 按需拉起辅助进程、客户端证书预读、加密会话票据与 IPC 客户端 | 凭据仅经鉴权 IPC 传输，不写入命令行或临时文件 |
| [`internal/privilege`](../../internal/privilege/README.md) | 受保护辅助组件安装、完整性与权限校验、系统授权、平台服务、进程查询与运行路径 | 不依赖 `vpn`、`update`、`fortivpn` 或 GUI |
| [`internal/ipc`](../../internal/ipc/README.md) | 本地 IPC 长度前缀帧协议与调用方身份（UID/SID）及令牌摘要校验 | 传输层强校验对端身份与单帧大小上限 |
| [`internal/credentials`](../../internal/credentials/README.md) | 本地凭据加密密封器与平台主密钥保护 | 供 `profile` 与 `helperclient` 复用，不依赖上层业务 |
| [`internal/profile`](../../internal/profile/README.md) | 配置增删改查、字段校验与密码加密存储 | 不感知运行期隧道会话 |
| [`internal/storage`](../../internal/storage/README.md) | `~/.tunnelkit` 目录布局、原子文件写入与日志尾部脱敏读取 | 不依赖上层业务模型 |
| [`internal/redact`](../../internal/redact/README.md) | 日志写入与读取共用的凭据脱敏 | 仅依赖标准库，不承担网络拓扑匿名化 |
| [`internal/update`](../../internal/update/README.md) | 版本更新检查、多源降级、语义化版本比较与三端原子自更新 | 零业务耦合，严格校验发布页域名与安装包 SHA-256 |
| [`fortivpn`](../../fortivpn/README.md) | 纯 Go Fortinet SSL-VPN 协议内核与独立 CLI | 严禁引用 `tunnelkit/internal/**` 或 Wails |

## 目录结构

```text
tunnelkit/
├── main.go                    # Wails 桌面应用入口
├── cmd/
│   └── tunnelkit-helper/      # 按会话提权的辅助程序入口
├── fortivpn/                  # 独立纯 Go Fortinet SSL-VPN 内核
│   ├── cmd/fortivpn/          # 独立命令行客户端
│   ├── engine/                # 会话编排、自动重连与双向数据转发
│   ├── fortitest/             # 内存假网关与内存虚拟网卡（供集成测试）
│   └── internal/
│       ├── sslvpn/            # HTTPS 控制面认证、隧道配置与二进制帧协议
│       ├── ppp/               # 数据面 PPP 协议栈（LCP、IPCP、Echo）
│       ├── netdev/            # 三端 TUN 虚拟网卡适配
│       └── netcfg/            # 平台路由、分流 DNS 与异常恢复
├── internal/
│   ├── api/                   # Wails 绑定服务
│   ├── app/                   # 窗口配置、菜单栏托盘、自更新协调与生命周期装配
│   ├── credentials/           # 本地凭据加密密封器与平台密钥保护
│   ├── ipc/                   # 本地 IPC 帧协议与调用方身份校验
│   ├── helper/                # 特权辅助服务端、异步事件流与脱敏日志器
│   ├── helperclient/          # 辅助进程按需拉起、客户端封装与加密会话票据
│   ├── privilege/             # 特权组件安装、校验、授权与受保护运行目录
│   ├── profile/               # 配置模型、校验器、持久化与密码库
│   ├── redact/                # 日志写入与读取共用的凭据脱敏
│   ├── storage/               # 数据目录路径与原子写入
│   ├── update/                # 版本更新检查、多源回退、版本比较与原子自更新
│   └── vpn/                   # 桌面连接状态机、维护门禁与会话恢复
├── assets/
│   ├── brand/                 # 品牌母版与图标生成参数
│   └── icons/                 # 窗口与三端托盘内嵌图标
├── frontend/                  # Vue 3 前端工程
├── build/                     # Wails 构建配置、平台图标与打包脚本
├── scripts/                   # 版本提取、Windows 驱动准备与 CI/Release 契约测试
├── .github/                   # PR 模板、Dependabot 与 CI / Release / Rolling 工作流
├── docs/guide/                # 稳定项目指南
├── CHANGELOG.md               # 版本更新日志
└── Taskfile.yml               # 根级开发与打包任务入口
```
