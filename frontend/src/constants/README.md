# 静态常量 (`frontend/src/constants`)

集中管理跨模块共享的语义色通道、连接状态元数据、表单选项与存储键常量。

## 目录结构

- `tone.ts`：6-Tone（`neutral | brand | info | success | warning | danger`）语义色映射表。
- `appearance.ts`：主题模式（跟随系统、浅色、深色）常量。
- `profile.ts`：默认端口、认证方式、路由策略与 DNS 策略选项元数据。
- `vpn.ts`：连接状态与阶段文案。
- `log.ts`：日志级别筛选器、轮询间隔与渲染行数上限。
- `releases.ts`：GitHub Releases 地址与外链域名白名单。
- `storage.ts`：`localStorage` 持久化键名常量。

## 开发规范

- **纯静态定义**：仅包含不可变常量与纯查找函数，禁止引入响应式状态、副作用或上层模块。
