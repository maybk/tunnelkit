# 通用复合组件 (`frontend/src/components/common`)

基于 `src/components/base/`、`src/components/ui/` 与语义色通道封装的跨业务复用复合组件。

## 目录结构

- `HelperMissingBanner.vue`：内置辅助程序缺失时的紧凑提示与检测操作。
- `StateText.vue`：状态文字即时替换并轻量淡入，可通过稳定键避免时长逐秒重复动画。
- `StatusDot.vue`：6-Tone 状态指示圆点（支持脉冲动画）。
- `StatusBadge.vue`：6-Tone 语义状态胶囊徽标。
- `InlineAlert.vue`：6-Tone 内联提示横幅（支持标题、描述与自定义槽位）。
- `CopyButton.vue`：剪贴板一键复制按钮与「已复制」状态反馈。
- `PageTitle.vue`：工作区页面标题与父级面包屑组件。
- `SegmentedControl.vue`：分段单选控制器。
- `FieldGroup.vue`：表单字段容器（统一标签、描述与校验错误提示）。
- `AppDialog.vue`：通用模态弹窗容器。
- `ConfirmDialog.vue`：二次确认对话框。
- `EmptyState.vue`：空状态引导面板。
- `NoticeToast.vue`：右下角浮动提示，沿用原单条显示方式，轻量进出过渡可被新消息打断。
- `UpdateDialog.vue`：版本更新详情与下载安装进度弹窗。
- `AboutDialog.vue`：「关于 TunnelKit」应用信息、辅助程序状态与仓库跳转弹窗。
- `BrandMark.vue`：随浅色/深色主题自动切换的透明品牌标识。

## 开发规范

- **纯粹复用**：通过 Props 与 Emits 通信，不直接依赖具体路由页面状态。
