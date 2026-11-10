# 状态管理 (`frontend/src/stores`)

基于 Pinia Setup Store 管理前端全局响应式状态、事件流订阅与后端写操作。

## 目录结构

- `useNoticeStore.ts`：全局通知提示队列与自动消失计时。
- `useAppearanceStore.ts`：三态主题模式（`system / light / dark`）切换、持久化与 DOM `.dark` 类名同步。
- `useDesktopWindowStore.ts`：三端桌面平台探测与窗口控制动作。
- `useProfilesStore.ts`：VPN 节点列表、当前选中节点、工作区标签页与配置增删改查。
- `useVpnStore.ts`：VPN 实时状态、`vpn:state` 事件订阅、连接切换确认、挑战应答与证书信任重连。
- `useLogsStore.ts`：节点日志快照、在途读取去重、清空后的旧快照失效与剪贴板脱敏复制；轮询生命周期由日志面板管理。
- `useSystemStore.ts`：应用版本信息、更新检查、自更新进度订阅与发布页安全跳转。

## 开发规范

- **防腐调用**：所有宿主交互统一通过 `src/api/*` 完成，禁止直接导入 `@bindings/*`、`@wailsio/runtime` 或视图组件。
- **操作状态**：`busyAction` 保留到 API 返回；`pendingAction` 在对应事件确认后清除，避免终态重新显示加载图标。
- **状态顺序**：按 `revision` 接收查询与推送；应答、取消及请求状态与挑战 ID 关联。
