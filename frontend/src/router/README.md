# 客户端路由 (`frontend/src/router`)

管理桌面工作台与新建节点页面的声明式路由配置。

## 目录结构

- `routes.ts`：路由名称常量与路由表定义（`/workspace` 与 `/profiles/new`）。
- `index.ts`：基于 `createWebHashHistory()` 的 Vue Router 实例。

## 开发规范

- **按需加载**：路由页面统一采用动态 `import()` 懒加载，避免与视图组件产生循环求值。
