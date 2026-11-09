# 基础 UI 控件 (`frontend/src/components/ui`)

由 `shadcn-vue` 官方模板生成并同步的基础 UI 控件目录（配置定义于 `frontend/components.json`，底层基于 `reka-ui`）。

## 目录结构

- `alert/`：内联提示框控件。
- `alert-dialog/`：模态确认对话框控件。
- `badge/`：徽标控件。
- `button/`：按钮控件。
- `card/`：卡片容器控件。
- `checkbox/`：复选框控件。
- `dialog/`：模态弹窗控件。
- `dropdown-menu/`：下拉菜单控件。
- `input/`：单行文本输入框控件。
- `label/`：表单标签控件。
- `popover/`：弹出浮层控件。
- `radio-group/`：单选框组控件。
- `scroll-area/`：自定义滚动区域控件。
- `select/`：下拉选择器控件。
- `separator/`：分隔线控件。
- `sheet/`：侧边抽屉控件。
- `skeleton/`：骨架屏占位控件。
- `switch/`：开关切换控件。
- `tabs/`：标签页切换控件。
- `textarea/`：多行文本输入框控件。
- `tooltip/`：文字提示控件。

## 开发规范

- **官方生成**：新增或更新控件统一依据 `frontend/components.json` 通过 `shadcn-vue` 官方 CLI 生成同步，不手写基础控件。
- **最少修改**：保持与 `shadcn-vue` 官方模板一致以便平滑升级；业务复合封装统一放在 `src/components/common/` 或 `src/components/<domain>/`。
- **依赖约束**：仅允许依赖 `@/lib/*`、同级 `@/components/ui/*` 及第三方 UI 库，严禁导入业务模块。
