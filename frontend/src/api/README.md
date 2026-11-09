# 宿主防腐适配层 (`frontend/src/api`)

封装 Wails 生成绑定与桌面运行时调用，将传输 DTO 转换为前端领域类型。

## 目录结构

- `mappers.ts`：Wails 生成 DTO 与前端领域类型双向映射器。
- `profile.ts`：连接配置增删改查、证书指纹信任与配置文本转换接口。
- `vpn.ts`：VPN 连接启停、挑战应答、日志读取与 `vpn:state` 事件订阅接口。
- `system.ts`：系统信息、版本检查、自更新执行与 `update:progress` 事件订阅接口。
- `window.ts`：三端桌面窗口探测、最小化、最大化/还原与隐藏接口。

## 开发规范

- **防腐隔离**：仅本目录允许导入 `@bindings/*` 与 `@wailsio/runtime`，对外统一暴露 `src/types/*` 纯领域类型。
- **单向依赖**：禁止反向依赖 `stores/`、`composables/`、`components/`、`views/`、`layouts/`、`router/` 或 `presentation/`。
