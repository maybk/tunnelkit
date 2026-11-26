# 工作台布局壳层 (`frontend/src/layouts`)

承载桌面窗口三端融合顶栏、左侧节点导航栏与主工作台双栏骨架。

## 目录结构

- `windowRegions.ts`：Wails v3 窗口拖拽区域（`--wails-draggable`）与 Windows 非客户区命中样式（`--wails-non-client-region`）。
- `DesktopChrome.vue`：三端自适应分栏顶栏，展示应用标题、当前页标题/面包屑及 Windows 窗口控制键。
- `AppSidebar.vue`：左侧 VPN 节点列表、标题行新建入口、快捷连接操作与底部设置入口。
- `WorkbenchLayout.vue`：根布局壳层，编排顶栏、主侧栏与设置分区整栏切换、主视口及全局弹窗。

## 开发规范

- **三端一致性**：macOS 非全屏模式预留左侧红绿灯安全间距，Windows 无边框模式渲染自定义最小化/最大化/关闭按钮并声明 `--wails-non-client-region`。
