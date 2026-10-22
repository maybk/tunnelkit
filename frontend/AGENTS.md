# TunnelKit 前端

## 模块边界

- `src/main.ts` 与 `src/App.vue`：入口挂载与顶层布局装配（`DesktopChrome` + `ApplicationBody` + `AboutDialog` + `NoticeToast`）。
- `src/api/`：唯一直接导入 Wails `bindings/` 与 `@wailsio/runtime` 的目录（`profile.ts`、`vpn.ts`、`system.ts`、`window.ts`）。
- `src/stores/`：基于 `@vueuse/core` `createGlobalState` 的全局状态（`useDesktopWindow`、`useSystem`、`useProfiles`、`useVpn`、`useNotice`）。
- `src/views/`：页面级视图（`ProfileView`、`CreateProfileView`、`AboutDialog`）。
- `src/components/`：按领域划分为 `sidebar/`、`connection/`、`profile/`、`common/` 和 `ui/`（shadcn-vue 基础控件）。
- `src/lib/`：`status.ts`（`profileStatus()` 状态映射唯一来源）、`format.ts`（时长、流量与错误格式化）与 `utils.ts`（`cn()` 类名合并）。
- `src/assets/main.css`：浅色/深色主题变量、字号 token 与桌面标题栏几何常量（`--chrome-height: 52px`）。

## 状态与交互规则

1. 后端 `vpn:state` 事件与 API 响应是连接状态的唯一事实源；前端只维护已连接项的时长秒表，不轮询进程状态。
2. 字段校验以后端 `profile.NormalizeAndValidate` 为唯一来源；前端只做首尾去空白与原生 `required` 校验。
3. 错误分流：连接失败写入 `state.error` 并在 `ErrorCard` 展示；表单保存/删除错误在 `ProfileForm` 内展示；其余操作错误通过 `useNotice` Toast 展示。
4. `ProfileForm.vue` 通过 `src/lib/configText.ts` 保持「表单填写」与「配置文本」双向实时同步，保留注释行与空行；已保存密码且未输入新密码时只显示 `# password = ******** (已加密保存)`，绝不回显明文。
5. 禁止手工编辑 `bindings/`（由根目录 `wails3 generate bindings -ts` 生成）；禁止调用 `window.alert / confirm / prompt`（使用 `AlertDialog` / `Dialog`）。
6. 自定义字号类（`text-caption`、`text-body`、`text-title`、`text-heading`）必须在 `src/lib/utils.ts` 的 `extendTailwindMerge` `font-size` 分组中注册，防止 `cn()` 覆盖文字颜色类。
7. 顶栏 `DesktopChrome` 使用 `--wails-draggable: drag`，内部交互控件必须标记 `no-drag`，macOS 下保留左侧交通灯安全间距 `pl-[78px]`。
8. 测试位于 `tests/*.spec.ts`（Vitest + `@vue/test-utils`）；修改后运行 `pnpm run format:check`、`pnpm run lint:check`、`pnpm run test` 与 `pnpm run build` 验证。
