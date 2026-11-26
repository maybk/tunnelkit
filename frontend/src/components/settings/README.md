# 设置页组件 (`frontend/src/components/settings`)

提供设置分区的专用左侧导航栏、分组卡片与设置行容器。

## 目录结构

- `SettingsSidebar.vue`：设置分区左侧导航栏，提供返回入口与二级菜单切换。
- `SettingCard.vue`：设置分组卡片容器，统一渲染分组标题、表面阴影与行间分隔线。
- `SettingRow.vue`：单条设置行容器，左侧展示标题与说明，右侧承载表单控件。

## 开发规范

- **视觉一致**：`views/settings/` 下的全部子视图统一复用本目录容器，保持行距与分隔线密度一致。
