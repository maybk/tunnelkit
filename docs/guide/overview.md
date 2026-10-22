# 项目概览与目录结构

TunnelKit 是基于 Wails v3、Go 与 Vue 3 + TypeScript 构建的跨平台（macOS / Windows / Linux）Fortinet SSL-VPN 桌面与菜单栏客户端。

## 架构

```text
Vue 前端 -> Wails bindings -> internal/api -> internal/vpn -> internal/helperclient -> tunnelkit-helper -> fortivpn
```

| 模块 | 职责 | 边界约束 |
| --- | --- | --- |
| [`frontend/src`](../../frontend/README.md) | 桌面工作台界面、表单与配置文本双向编辑、认证弹窗、状态与日志展示 | 不直接读写本地文件、不保存密码明文 |
| [`internal/app`](../../internal/app/README.md) | 应用启动装配、桌面标题栏配置、菜单栏常驻与事件桥接 | 不承载连接状态机或配置校验逻辑 |
| `internal/api` | 暴露给前端的 Wails 服务适配层 | 负责参数转发、配置文本双向转换与状态拼装 |
| [`internal/vpn`](../../internal/vpn/README.md) | 桌面端连接状态机、交互式挑战桥接、连接切换与网络异常恢复 | 通过 `helperclient` 控制后台辅助进程 |
| [`internal/helper`](../../internal/helper/README.md) | 单会话特权辅助服务、脱敏日志落盘与网络恢复看门狗 | 仅响应通过本机身份与一次性令牌校验的连接 |
| `internal/profile` | 配置增删改查、字段校验与密码加密存储 | 不感知运行期隧道会话 |
| `internal/storage` | `~/.tunnelkit` 目录布局与原子文件写入 | 不依赖上层业务模型 |
| `internal/update` | GitHub Releases 版本更新检查、多源降级与语义化版本比较 | 零业务耦合，严格校验发布页跳转地址 |
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
│       ├── sslvpn/            # 控制面（登录、双因素认证、SAML、隧道配置）
│       ├── ppp/               # 数据面 PPP 协议栈（LCP、IPCP、Echo）
│       ├── netdev/            # macOS / Windows / Linux 虚拟网卡适配
│       └── netcfg/            # 平台路由、分流 DNS 与异常恢复
├── internal/
│   ├── api/                   # Wails 绑定服务
│   ├── app/                   # 窗口配置、菜单栏托盘与生命周期装配
│   ├── ipc/                   # 本地 IPC 帧协议与调用方身份校验
│   ├── helper/                # 特权辅助服务端与脱敏日志器
│   ├── helperclient/          # 辅助进程按需拉起与客户端封装
│   ├── profile/               # 配置模型、校验器、持久化与密码库
│   ├── storage/               # 数据目录路径与原子写入
│   ├── update/                # 版本更新检查、多源回退与版本比较
│   └── vpn/                   # 桌面连接状态机与会话恢复
├── assets/
│   ├── brand/                 # 品牌与菜单栏图标 SVG 母版
│   └── icons/                 # 桌面端与系统托盘内嵌 PNG 图标
├── frontend/                  # Vue 3 前端工程
├── build/                     # Wails 构建配置、平台图标与打包脚本
├── scripts/                   # 版本提取、Windows 驱动准备与 CI/Release 契约测试
├── .github/                   # PR 模板、Dependabot 与 CI / Release / Rolling 工作流
├── docs/guide/                # 稳定项目指南
├── CHANGELOG.md               # 版本更新日志
└── Taskfile.yml               # 根级开发与打包任务入口
```
