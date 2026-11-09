# TunnelKit 前端

## 模块边界

- `src/main.ts` 与 `src/App.vue`：入口挂载 Vue、Pinia、Vue Router 与工作台根组件。
- `src/api/`：唯一直接导入 Wails `bindings/` 与 `@wailsio/runtime` 的目录（`profile.ts`、`vpn.ts`、`system.ts`、`window.ts`）。
- `src/stores/`：Pinia Setup Stores（连接、配置、日志、系统更新、外观与窗口状态）。
- `src/views/`：主工作区与新建配置路由视图。
- `src/components/`：按 VPN、配置、日志、通用组合、项目基础适配 `base/` 与官方模板 `ui/` 划分。
- `src/domain/`、`src/validate/`、`src/presentation/`：纯领域转换、运行时输入校验与展示映射。
- `src/lib/`：第三方库与技术适配；`src/utils/`：通用计算与 DOM 判定。
- `src/styles/`：Tailwind CSS v4 与语义令牌，动效不延迟真实连接状态。

## 状态与交互规则

1. 后端 `vpn:state` 事件与 API 响应是连接状态的唯一事实源；连接、取消、断开和 OTP 应答必须有可独立收敛的 busy/error 状态。
2. 字段校验由后端规则保障，前端用 Valibot 在字段附近提供即时且一致的反馈。
3. 在 Tabs 根组件设置保留内容，配置 Tab 切换必须保留草稿；文本与表单转换保留有效字段及已保存密码状态，未知指令按解析结果展示警告。
4. SAML 与 FortiToken/OTP 交互遵循后端 prompt 契约，transport failure 后错误可见并允许重试。
5. 检查更新放在 About 弹窗，保持工作区简洁。
6. 禁止手工编辑 `bindings/`；禁止调用 `window.alert / confirm / prompt`（使用 Dialog）。
7. 桌面顶栏遵循 Wails 拖拽/non-client 区域约定，三端原生运行需分别验收。
8. 测试位于 `tests/`；修改后运行 format、lint、Vitest、build，并运行 `go test -race ./...` 与 `go vet ./...`。
