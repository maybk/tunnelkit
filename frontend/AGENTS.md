# TunnelKit 前端

## 模块边界

- `src/main.ts` 与 `src/App.vue`：入口挂载 Vue、Pinia、Vue Router 与工作台根组件。
- `src/api/`：唯一直接导入 Wails `bindings/` 与 `@wailsio/runtime` 的目录（`profile.ts`、`vpn.ts`、`system.ts`、`window.ts`）。
- `src/stores/`：Pinia Setup Stores（连接、配置、日志、系统更新、外观与窗口状态）。
- `src/views/`：主工作区与新建配置路由视图。
- `src/components/`：按 VPN、配置、日志、通用复合组件 `common/`、项目基础适配 `base/` 与官方模板 `ui/` 划分。
- `src/domain/`、`src/validate/`、`src/presentation/`：纯领域转换、运行时输入校验与展示映射。
- `src/lib/`：第三方库与技术适配；`src/utils/`：通用计算与 DOM 判定。
- `src/styles/`：Tailwind CSS v4 与语义令牌，动效不延迟真实连接状态。

## 编码与命名规范

- **命名约定**：页面 `*View.vue`，弹窗 `*Dialog.vue`，Store `use*Store.ts`（Setup 写法，ID 使用 `kebab-case`），组合式函数 `use*.ts`，静态常量 `UPPER_SNAKE_CASE`。
- **Vue 与类型约束**：统一使用 `<script setup lang="ts">`、`defineModel()` 与 `useTemplateRef()`；严禁使用 `v-html`、原生 `alert / confirm / prompt`、`console.log / debug`、`createGlobalState`、`as unknown as`、手写 `Raw*` 接口、`get label()` 访问器或预拼 `badgeClass`/`textClass`/`pillClass` 字段。
- **文件行数与拆分边界**：仅对 `src/` 下 `.vue <= 300` 行、`src/` 与 `config/` 下 `.ts`/`.js`/`.mjs`/`.cjs <= 400` 行、`tests/` 下同类脚本 `<= 800` 行设限（豁免 `src/components/ui/`、`tests/fixtures/` 与 `*.d.ts`；其余目录与后缀不限）。超限时 `.vue` 优先抽 Composable 或领域子组件（高频列表行不拆多层空壳），测试按场景拆分并将大夹具迁入 `tests/fixtures/`；禁止删注释、压行或无语义拆分命名。

## 状态与交互规则

1. 后端 `vpn:state` 事件与 API 响应是连接状态的唯一事实源；连接、取消、断开和 OTP 应答必须有可独立收敛的 busy/error 状态。
2. 字段校验由后端规则保障，前端用 Valibot 在字段附近提供即时且一致的反馈。
3. 在 Tabs 根组件设置保留内容，配置 Tab 切换必须保留草稿；文本与表单转换保留有效字段及已保存密码状态，未知指令按解析结果展示警告。
4. SAML 与 FortiToken/OTP 交互遵循后端 prompt 契约，transport failure 后错误可见并允许重试。
5. 检查更新放在 About 弹窗，保持工作区简洁；禁止手工编辑 `bindings/`。
6. 桌面顶栏遵循 Wails 拖拽/non-client 区域约定，三端原生运行需分别验收。
7. 测试位于 `tests/`；修改后运行 format、lint、Vitest、build，并运行 `go test -race ./...` 与 `go vet ./...`。

## 文档维护

- `README.md` 只做前端入口、快速开始和文档导航。
- 架构、配置、开发、样式与测试规范分别维护在 `docs/guide/` 指南与各目录 `README.md` 中；目录 `README.md` 只写职责、边界与规范，不枚举具体函数、方法或常量名；相关目录或结构变更时同步更新。
