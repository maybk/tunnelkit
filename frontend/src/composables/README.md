# 组合式逻辑 (`frontend/src/composables`)

封装跨组件复用的交互编排、启动初始化、全局快捷键与表单草稿状态。

## 目录结构

- `useAppBootstrap.ts`：应用启动期主题、窗口、系统信息、节点列表与 VPN 事件流初始化。
- `useProfileFormDraft.ts`：节点表单草稿状态、脏检查、模式切换与 Valibot 提交校验。
- `useDisconnectFeedback.ts`：断开按钮最短 350ms 的展示反馈，独立于网络状态；失败、切换节点或卸载时清理。
- `useGlobalShortcuts.ts`：桌面全局快捷键绑定（排除可编辑输入框焦点冲突）。

## 开发规范

- **依赖边界**：可调用 `stores/`、`validate/`、`domain/`、`presentation/` 与 `utils/`，禁止直接导入 `@/api/*`、`@/views/*`、`@/components/*` 或 `@/layouts/*`。
