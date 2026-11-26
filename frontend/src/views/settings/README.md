# 设置二级子视图 (`frontend/src/views/settings`)

承载设置分区（`/settings/:tab`）下的二级子页面。

## 目录结构

- `GeneralView.vue`：通用设置子页，管理启动时自动检查更新等常规偏好。
- `AppearanceView.vue`：外观设置子页，管理跟随系统、浅色与深色主题模式。
- `AboutView.vue`：关于子页，展示版本信息、后台隧道服务状态、数据目录路径与复制、更新检查及开源仓库入口。

## 开发规范

- **布局统一**：使用设置分组卡片与行容器组织设置项，页面标题统一由顶栏展示。
- **依赖约束**：通过 `stores/` 读写状态，禁止直接导入 `@/api/*`、`@bindings/*` 或 `@wailsio/runtime`。
