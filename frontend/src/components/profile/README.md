# 节点配置表单组件 (`frontend/src/components/profile`)

承载 VPN 节点配置的可视化分区表单与配置文本双模编辑。

## 目录结构

- `ProfileBasicSection.vue`：网关地址、端口与地址粘贴解析分区。
- `ProfileAuthSection.vue`：用户名、加密保存密码状态与密码输入分区。
- `ProfileAdvancedSection.vue`：认证方式、Realm、证书指纹、客户端证书及旧版网关兼容分区。
- `ProfileNetworkSection.vue`：路由策略、附加 CIDR、DNS 策略与自动重连分区。
- `ProfileFormActions.vue`：新建配置时的保存与取消操作；编辑操作位于工作台配置标签栏。
- `ProfileFormPanel.vue`：组合上述分区与配置文本编辑器的完整表单面板。

## 开发规范

- **草稿解耦**：表单草稿同步、Valibot 校验与文本双向转换由 `useProfileFormDraft` 统一编排。
