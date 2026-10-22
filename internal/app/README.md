# 窗口、顶栏与菜单栏集成 (`internal/app`)

本包负责 Wails v3 桌面应用装配、平台自适应无边框顶栏配置、系统菜单栏（Tray）常驻及前后端状态同步。

## 职责

- [app.go](app.go)：初始化存储、配置与 VPN 服务，注册 Wails 绑定并将 `vpn.State` 变更实时推送至前端事件 `vpn:state` 与菜单栏。
- [window.go](window.go)：配置主窗口尺寸（默认 `960×640`，最小 `780×520`）与平台原生窗口样式（macOS `MacTitleBarHiddenInset` 保留红绿灯，Windows/Linux `Frameless` 由前端渲染自绘窗口控制键），拦截窗口关闭事件隐藏至菜单栏并在 macOS 下自动释放 Dock 占用。
- [tray.go](tray.go)：管理系统菜单栏图标与快捷菜单（仪表板、节点切换、连接开关、退出），退出前自动断开活跃隧道并恢复网络。

## 验证

```bash
go test -race ./internal/app/...
```
