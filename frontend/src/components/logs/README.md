# 连接日志组件 (`frontend/src/components/logs`)

承载节点连接日志的随连接状态实时刷新、手动刷新、脱敏复制与清空操作。

## 目录结构

- `LogToolbar.vue`：显示日志更新时间与实时状态，提供手动刷新、复制与清空操作。
- `LogStreamList.vue`：终端式原始日志文本与自动吸底滚动容器。
- `LogViewerPanel.vue`：组合日志工具栏与日志流的完整查看面板。

## 开发规范

- **脱敏导出**：复制或导出日志时必须经过 `utils/redact.ts` 的敏感字段脱敏处理。
