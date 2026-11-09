# 前端测试套件 (`frontend/tests`)

存放前端纯函数单测、Wails DTO 映射测试、Pinia Store 测试、Composable 测试、组件/视图集成测试与架构静态守卫测试。

## 目录结构

- `setup.ts`：集成测试环境初始化入口（Mock `@wailsio/runtime`、装配独立 Pinia 与 Vue Router 实例）。
- `fixtures/`：测试共用的节点配置与 VPN 状态构造夹具。
- `architecture/`：前端目录分层、单向依赖、技术适配层边界与不安全代码模式静态守卫测试。
- `unit/`：面向 `utils/`、`domain/`、`validate/` 与 `presentation/` 的单元测试。
- `api/`：DTO 转换与外链安全边界测试。
- `stores/`：面向 `src/stores/*` 状态流转与宿主 API 交互的测试。
- `composables/`：草稿、双模转换、启动容错及断开反馈计时测试。
- `components/`：基础组件契约、三端顶栏、弹窗、连接/断开反馈与二次认证交互测试。
- `views/`：面向 `WorkspaceView` 的工作区集成测试。

## 开发规范

- **分层执行**：`unit/` 与 `architecture/` 在 `unit` 项目中快速运行，`api/`、`stores/`、`composables/`、`components/` 与 `views/` 在挂载 `setup.ts` 的 `integration` 项目中隔离运行。
