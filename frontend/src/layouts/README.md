# 工作台布局壳层 (`frontend/src/layouts`)

承载桌面窗口三端融合顶栏、左侧节点导航栏与主工作台双栏骨架。

## 目录结构

- `windowRegions.ts`：Wails v3 窗口拖拽区域（`--wails-draggable`）与 Windows 非客户区命中样式（`--wails-non-client-region`）。
- `DesktopChrome.vue`：macOS / Windows / Linux 自适应桌面顶栏（连接状态、主题切换、关于入口与窗口控制键）。
- `AppSidebar.vue`：左侧 VPN 节点列表、状态圆点、快捷连接操作与新建按钮。
- `WorkbenchLayout.vue`：组合桌面顶栏、侧边栏、主路由视口与全局弹窗的根布局壳层。

## 开发规范

- **三端一致性**：macOS 非全屏模式预留左侧红绿灯安全间距，Windows 无边框模式渲染自定义最小化/最大化/关闭按钮并声明 `--wails-non-client-region`。
