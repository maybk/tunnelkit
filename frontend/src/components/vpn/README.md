# VPN 状态与认证组件 (`frontend/src/components/vpn`)

承载 VPN 连接状态控制、会话信息与二次认证交互。

## 目录结构

- `VpnConnectionHero.vue`：节点连接状态主卡片与一键连接/断开控制。
- `VpnLiveMetrics.vue`：实时隧道、流量卡片与退出过渡。
- `VpnSessionMetricsGrid.vue`：连接概览、已连接隧道地址与流量，以及网关、身份、证书、网络策略四项摘要。
- `VpnChallengeDialog.vue`：OTP 验证码、FortiToken 推送确认与 SAML 浏览器认证弹窗。

## 开发规范

- 组件负责状态展示与事件分发，连接动作交由 `useVpnStore` 执行。
- 连接文案共用 `presentation/vpn.ts`，状态点显式传入 `pulse`。
- 请求反馈与防重锁分离；按钮的短暂收尾效果不延迟真实状态。
