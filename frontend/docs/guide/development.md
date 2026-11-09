# 开发规范与工作流

## 常用命令

在仓库根目录执行：

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run format:check
pnpm --dir frontend run lint:check
pnpm --dir frontend run test -- --run
pnpm --dir frontend run build
```

或在 `frontend/` 目录下直接使用 Vite+ CLI：

```bash
pnpm exec vp check
pnpm exec vp test run
pnpm run build
```

## 代码规范

1. **组件与文件体积**：
   - 组件统一使用 `<script setup lang="ts">`，双向绑定优先使用 `defineModel()`。
   - 按职责和复用需求拆分模块。
2. **状态与表单校验**：
   - 跨组件共享业务状态使用 Pinia Setup Store（`src/stores/use*Store.ts`）；局部状态由组件或 composable 管理。
   - 表单草稿、OTP 验证码、缓存与外链由 `src/validate/` 提供可复用校验。外部数据在适配入口解析并校验，处理解析失败，不以 TypeScript 断言代替运行时检查。
3. **安全红线**：
   - 严禁使用 `v-html` 渲染任何日志、网关提示或更新说明。
   - VPN 密码、Cookie、Token 与 OTP 仅用于当前交互和调用，不写入浏览器持久化存储。
   - 日志复制与导出必须经过 `src/utils/redact.ts` 脱敏。
4. **三端桌面顶栏适配**：
   - `DesktopChrome.vue` 与 `windowRegions.ts` 使用 `--wails-draggable: drag | no-drag` 控制拖拽区，Windows 窗口控制按钮标注 `--wails-non-client-region: minimize | maximize | close`。
