# 领域类型定义 (`frontend/src/types`)

定义前端各层共用的纯 TypeScript 接口与类型别名，与 Wails 生成绑定解耦。

## 目录结构

- `appearance.ts`：主题模式与本地外观缓存类型。
- `desktop.ts`：桌面宿主平台枚举类型。
- `profile.ts`：VPN 节点配置实体、表单草稿、提交载荷与文本解析结果类型。
- `vpn.ts`：VPN 连接状态、握手阶段、隧道参数、流量统计、挑战弹窗与错误类型。
- `log.ts`：日志视图快照、结构化日志行与级别筛选类型。
- `system.ts`：系统信息、版本更新详情与下载安装进度类型。

## 开发规范

- **纯类型文件**：仅允许包含 `interface`、`type` 与 `import type` / `export type`，严禁包含运行时代码。
