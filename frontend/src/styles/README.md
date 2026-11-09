# 样式与设计令牌 (`frontend/src/styles`)

维护 Tailwind CSS v4 入口、6-Tone 语义颜色令牌、字体阶梯与桌面全局样式。

## 目录结构

- `index.css`：样式总入口，按序导入 Tailwind CSS、`tw-animate-css`、设计令牌与基础层样式。
- `base.css`：桌面 WebView 基础排版、全局重置、统一隐藏原生滚动条与减少动态效果规则。
- `utilities.css`：表面卡片、桌面拖拽区域工具类。
- `tokens/`：基础调色板、6-Tone 语义色变量、尺寸字号阶梯与 Tailwind `@theme inline` 映射定义。

## 开发规范

- **语义令牌优先**：业务组件优先使用语义色令牌（`bg-card`、`text-muted-foreground`、`tone-*`），避免硬编码裸十六进制色值。
