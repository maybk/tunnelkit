# TunnelKit 前端

TunnelKit 前端是运行在 Wails v3 WebView 中的 Vue 3 + TypeScript 桌面界面，负责连接配置管理、实时状态展示、错误诊断、日志查看以及桌面顶栏交互。

## 技术栈

- Vue 3.5、TypeScript 6.0、Vite 8
- Tailwind CSS v4、shadcn-vue、Lucide 图标
- `@vueuse/core`、`@wailsio/runtime` 与 Wails TypeScript bindings
- Vitest、`@vue/test-utils`、`happy-dom`、`oxlint`、`eslint`、`oxfmt`

## 快速开始

在 `frontend/` 目录安装依赖：

```bash
pnpm install
```

从仓库根目录启动完整桌面开发环境：

```bash
cd ..
wails3 dev
```

## 常用命令

在 `frontend/` 目录执行：

```bash
pnpm run format:check
pnpm run lint:check
pnpm run test
pnpm run build
```

## 项目文档

- [项目概览与目录结构](../docs/guide/overview.md)
- [配置说明](../docs/guide/configuration.md)
- [开发与构建](../docs/guide/development.md)
