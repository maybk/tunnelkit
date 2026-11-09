# TunnelKit 前端 (`frontend`)

TunnelKit 前端是运行在 Wails v3 WebView 中的 Vue 3 + TypeScript 桌面工作台，负责连接配置管理、实时状态监控、二次认证交互、日志排查与三端融合顶栏控制。

## 目录结构

- `index.html`：桌面 WebView 页面入口与首屏主题防闪烁脚本。
- `package.json`：前端依赖声明与构建、检查、测试脚本入口。
- `vite.config.ts`：Vite+ 构建、格式化、Oxlint 与测试配置。
- `tsconfig.json`：TypeScript 项目引用入口。
- `tsconfig.app.json`：前端源码与测试的类型检查配置。
- `tsconfig.node.json`：构建配置的类型检查配置。
- `pnpm-lock.yaml`：依赖版本锁文件。
- `pnpm-workspace.yaml`：依赖目录与版本覆盖配置。
- `components.json`：`shadcn-vue` 官方基础控件生成与同步配置。
- `config/`：Vite 构建期模块边界静态守卫插件。
- `docs/`：前端架构、开发、主题样式与测试指南。
- `public/`：静态图标资源。
- `src/`：前端工作台业务源码。
- `tests/`：单元、架构守卫与组件集成测试套件。

## 开发规范

- [前端概览与架构](./docs/guide/overview.md)
- [开发规范与工作流](./docs/guide/development.md)
- [主题与样式系统](./docs/guide/theme-style.md)
- [测试与架构守卫](./docs/guide/testing.md)

## 常用命令

在仓库根目录执行：

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run format:check
pnpm --dir frontend run lint:check
pnpm --dir frontend run test -- --run
pnpm --dir frontend run build
```
