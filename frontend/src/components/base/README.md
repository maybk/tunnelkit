# 项目基础组件

提供 TunnelKit 外观适配与基础交互组件，保持业务无关。

## 目录结构

- `button/`：按钮组件及其专用变体规则。
- `AppInput.vue`：输入框样式适配。
- `AppTextarea.vue`：多行输入框样式适配。
- `AppLabel.vue`：字段标签排版适配。
- `AppTabsList.vue`：标签条样式适配。
- `AppTabsTrigger.vue`：标签项与选中态样式。
- `AppDialogContent.vue`：弹窗遮罩、统一 sm/md/lg 宽度、视口高度约束及中文关闭按钮。
- `AppDialogTitle.vue`：弹窗标题排版适配。
- `AppDialogDescription.vue`：弹窗描述排版适配。
- `AppDropdownMenuContent.vue`：菜单浮层尺寸、阴影与滚动边界。
- `AppDropdownMenuItem.vue`：菜单项排版适配。

## 开发规范

1. 优先复用 `ui/`，通过主题令牌、变体、class 或插槽适配外观；需要改变结构时再定制源码。
2. 源码定制以 `ui/` 对应组件为升级基线，核对属性、事件、插槽及可访问性。
3. 正确转发模型、事件、属性和 ref，保留键盘、焦点、禁用及受控语义；Portal 属性绑定到实际内容节点。
4. 弹窗使用 `size` 选择宽度，受视口边距限制，内容过长时可滚动；滚动条由全局样式管理。
5. 仅依赖本目录、`ui/`、`lib/` 与 UI 基础库；跨业务组合放在 `common/`，业务交互放在领域组件。
