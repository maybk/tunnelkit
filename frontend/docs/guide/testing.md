# 测试与架构守卫

TunnelKit 前端测试基于 `vite-plus/test`（Vitest）与 `@vue/test-utils`，在 `vite.config.ts` 中划分为两个隔离项目：

1. **`unit` 项目**（`tests/{unit,architecture}/**/*.spec.ts`）：
   - `tests/architecture/architecture.spec.ts`：基于 TypeScript AST 静态校验分层导入方向、通用层零框架依赖、技术适配层边界、`types/` 纯类型约束、API/Store 无循环依赖及不安全渲染与双重断言。
   - `tests/unit/*.spec.ts`：覆盖 `utils/`、`domain/`、`validate/` 与 `presentation/` 的单元测试。
2. **`integration` 项目**（`tests/{api,stores,composables,components,views}/**/*.spec.ts`）：
   - 通过 `tests/setup.ts` 自动 Mock `@wailsio/runtime`，并在每个用例前创建独立 Pinia、将共享 Router 重置到工作区。
   - 覆盖 `api/mappers.ts` DTO 映射、`stores/*` 状态流转、`useProfileFormDraft` 双模草稿编排、`DesktopChrome` 三端适配与 `WorkspaceView` 工作台集成测试。

## 关键回归路径

- 连接 Promise 尚未返回时可取消；OTP 应答失败后释放门禁，SAML 自动打开与重新打开、FortiToken 推送空值继续均可用。
- 切换配置标签保留草稿；还原后拒绝过期的文本解析结果；保存门禁覆盖解析与校验过程。
- VPN 状态读取失败不阻断配置加载；更新进行中拒绝弹窗关闭；仓库/发版链接与网关认证链接采用各自的安全边界。
- 日志读取去重，清空后丢弃旧快照；仅在日志标签可见且当前节点连接中或已连接时轮询。

这些测试使用模拟宿主，不代替三端真实 WebView、系统授权、VPN 网关和原生退出验收。

## 运行测试

```bash
pnpm --dir frontend run test -- --run
```
