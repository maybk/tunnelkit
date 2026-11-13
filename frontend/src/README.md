# 前端工作台源码 (`frontend/src`)

承载桌面工作台的视图、状态管理、领域逻辑、数据校验与宿主 API 适配实现。

## 目录结构

- `main.ts`：前端应用挂载入口。
- `App.vue`：根组件，负责生命周期初始化与工作台布局挂载。
- `vite-env.d.ts`：Vite 客户端环境与 `.vue` 模块类型声明。
- `router/`：客户端路由表与导航配置。
- `views/`：顶层路由视图页面。
- `layouts/`：三端融合标题栏、侧边节点栏与工作台布局壳层。
- `components/`：按基础控件库、通用复合组件与业务域拆分的 UI 组件。
- `assets/`：经 Vite 打包的前端静态品牌资源。
- `composables/`：跨组件交互逻辑、快捷键与表单草稿编排。
- `stores/`：Pinia 状态模块，承接后端快照、事件流与状态写操作。
- `presentation/`：面向视图的展示文案、状态语义与错误信息转换。
- `domain/`：与框架无关的纯业务规则与模型计算。
- `validate/`：基于 Valibot 的表单、OTP 与本地缓存校验。
- `constants/`：连接阶段、语义色通道、外观与存储键等静态常量。
- `utils/`：格式化、网络地址解析、凭据脱敏与 DOM 判定。
- `api/`：Wails 宿主服务与事件防腐适配层。
- `lib/`：第三方库适配与跨模块技术基础设施。
- `styles/`：6-Tone 语义令牌与全局样式。
- `types/`：按领域拆分的共享 TypeScript 类型定义。

## 开发规范

- **单向依赖**：视图与组件通过 `composables/` 或 `stores/` 调用 `api/`；共享展示、校验与领域层不反向依赖 UI 或宿主，仅 `src/api/` 允许调用 `@bindings` 与 `@wailsio/runtime`。
- **按职责拆分**：按职责和复用需求提取模块。
