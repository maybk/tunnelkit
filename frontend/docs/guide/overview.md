# 前端概览与架构

TunnelKit 前端基于 Vue 3.5、TypeScript 6.0、Vite+、Pinia、Vue Router、Valibot 与 Tailwind CSS v4 + `shadcn-vue` 构建，运行于 Wails v3 桌面容器（macOS `WKWebView`、Windows `WebView2`、Linux `WebKitGTK`）中。

## 分层与依赖方向

主要依赖方向如下；视图层可直接使用 Store，共享层不反向依赖 UI 或 Wails。

```text
views / layouts / components -> composables / stores -> api -> Wails bindings / runtime
                                      |                |
                                      v                v
                    presentation / validate / domain / utils / constants / types
```

- **防腐隔离**：仅 `src/api/` 允许导入 `@bindings/*` 与 `@wailsio/runtime`，对外统一暴露 `src/types/*` 领域类型；由 `config/runtime-boundary.ts`（构建期）与 `tests/architecture/architecture.spec.ts`（测试期）双重守护。
- **基础控件同步**：`src/components/ui/` 由 `shadcn-vue` 官方模板生成并同步（配置定义于 `frontend/components.json`），不直接承载业务语义；项目外观适配和必要的最小源码定制位于 `src/components/base/`，无差异组件直接复用官方实现；业务复合封装统一置于 `src/components/common/` 或 `src/components/<domain>/`。
- **技术基础设施**：`src/lib/` 封装第三方库与技术能力，`src/utils/` 提供通用计算和 DOM 判定。
- **纯粹领域与校验**：`src/domain/`、`src/validate/`、`src/utils/` 与 `src/constants/` 保持零框架依赖（不导入 Vue、Pinia、Router 或宿主运行时）。

## 目录布局

```text
frontend/
├── index.html                  # WebView 入口，加载外置首屏主题脚本
├── components.json             # shadcn-vue 官方组件同步配置
├── vite.config.ts              # Vite+ 构建、格式化、Oxlint 与 Vitest 双项目配置
├── config/                     # Vite 构建期架构边界静态守卫插件
├── docs/guide/                 # 前端稳定开发指南
├── public/                     # 首屏主题脚本与静态图标
├── src/                        # 前端工作台源码
└── tests/                      # 单元、架构、Store、Composable 与组件测试
```
