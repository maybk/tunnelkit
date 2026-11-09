# 前端构建期配置 (`frontend/config`)

维护 Vite 构建期的架构边界静态守卫插件，不进入浏览器运行时业务包。

## 目录结构

- `runtime-boundary.ts`：Rollup/Vite 模块解析守卫，拦截 `src/api/*` 之外的业务代码直接导入 `@bindings/*` 或 `@wailsio/runtime`。

## 开发规范

- **构建期纯模块**：仅包含 Node/Vite 构建期插件与纯函数，不依赖 Vue、Pinia、浏览器 DOM 或 Wails 运行时。
