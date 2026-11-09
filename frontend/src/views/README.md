# 路由视图 (`frontend/src/views`)

承载客户端顶层路由页面，负责组合业务子组件与工作区标签页。

## 目录结构

- `WorkspaceView.vue`：主工作台视图（节点连接状态主卡、特权辅助程序告警、状态/配置/日志三标签页切换与配置保存）。
- `ProfileCreateView.vue`：新建 VPN 节点页面视图。

## 开发规范

- **薄视图编排**：页面文件聚焦布局与组件组合，表单校验与状态读写交由 `composables/` 与 `stores/` 处理。
