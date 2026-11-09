<script setup lang="ts">
import { computed } from 'vue'
import { AlertTriangle, Globe, Lock, ShieldCheck, SlidersHorizontal, User } from '@lucide/vue'
import CopyButton from '@/components/common/CopyButton.vue'
import StateText from '@/components/common/StateText.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import { getViewingStatusMeta, describeVpnPhase } from '@/presentation/vpn'
import { resolveViewingStatus } from '@/domain/vpn'
import type { Profile } from '@/types/profile'
import type { VpnState } from '@/types/vpn'
import VpnLiveMetrics from './VpnLiveMetrics.vue'
import { formatElapsedSince, formatClockTime } from '@/utils/time'
import { formatEndpoint } from '@/utils/network'

const props = defineProps<{ profile: Profile; state: VpnState; elapsedText: string }>()
const view = computed(() => ({
  ...getViewingStatusMeta(props.state, props.profile.id),
  status: resolveViewingStatus(props.state, props.profile.id),
}))
const endpoint = computed(() => formatEndpoint(props.profile.host, props.profile.port))
const sessionDuration = computed(
  () => props.elapsedText || formatElapsedSince(props.state.connectedAt),
)
const liveInfo = computed(() => (view.value.status === 'connected' ? props.state.info : null))
const liveStats = computed(() => (view.value.status === 'connected' ? props.state.stats : null))
const statusSummary = computed(() => {
  if (view.value.status === 'connected')
    return {
      title: '已建立安全连接',
      detail: props.state.connectedAt
        ? '连接建立于 ' + formatClockTime(props.state.connectedAt) + '，网络流量正通过当前 VPN 转发'
        : '网络流量正通过当前 VPN 转发',
    }
  if (view.value.status === 'connecting')
    return {
      title: describeVpnPhase(props.state, props.profile),
      detail: '若弹出系统权限或验证码窗口，请按提示完成确认',
    }
  if (view.value.status === 'disconnecting')
    return { title: '正在断开连接…', detail: '正在关闭隧道并恢复原有路由与 DNS 设置' }
  if (props.state.profileId === props.profile.id && props.state.error)
    return { title: '上次连接未成功', detail: '请根据上方提示检查配置后重新连接' }
  return { title: '当前未连接', detail: describeVpnPhase(props.state, props.profile) }
})
const routePolicySummary = computed(
  () =>
    ({
      none: '仅建立连接（不改路由）',
      full: '全局代理（所有流量）',
      gateway: '智能分流（仅内网流量）',
    })[props.profile.routes],
)
const dnsPolicySummary = computed(() => {
  const parts = [
    { off: '保持本机默认 DNS', full: '全局使用 VPN DNS', split: '智能分流 DNS' }[props.profile.dns],
  ]
  if (props.profile.dnsDomains.length)
    parts[0] += '（' + props.profile.dnsDomains.length + ' 个域名）'
  if (props.profile.extraRoutes.length)
    parts.push('附加路由 ' + props.profile.extraRoutes.length + ' 条')
  return parts.join(' · ')
})
</script>
<template>
  <div class="space-y-3.5">
    <div
      v-if="liveInfo?.splitDnsNeedsDomains"
      class="flex items-start gap-2.5 rounded-2xl border border-tone-warning-fg/35 bg-tone-warning-fg/[0.08] px-4 py-3 text-caption text-foreground"
    >
      <AlertTriangle class="mt-0.5 size-4 shrink-0 text-tone-warning-fg" aria-hidden="true" />
      <div>
        <span class="font-medium">提示：分流 DNS 尚未生效。</span>
        <span class="text-muted-foreground">
          当前网关未下发搜索域，请在「配置」页填写「内网域名后缀」或切换为「全局使用 VPN DNS」。
        </span>
      </div>
    </div>

    <!-- 当前连接状态概览 -->
    <div class="surface-card flex items-center justify-between gap-4 rounded-2xl px-5 py-4">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <StatusDot :tone="view.tone" :pulse="view.pulse" />
          <StateText :text="statusSummary.title" class="text-body font-semibold text-foreground" />
        </div>
        <p class="mt-1 text-caption text-muted-foreground">
          {{ statusSummary.detail }}
        </p>
      </div>

      <div v-if="view.status === 'connected' && sessionDuration" class="shrink-0 text-right">
        <div class="text-caption text-muted-foreground">持续时长</div>
        <div class="mt-0.5 font-mono text-title font-semibold text-tone-success-solid tabular-nums">
          {{ sessionDuration }}
        </div>
      </div>
    </div>

    <!-- 已连接实时隧道参数与流量 -->
    <VpnLiveMetrics :info="liveInfo" :stats="liveStats" />

    <!-- 2x2 配置摘要矩阵 -->
    <div class="grid grid-cols-2 gap-3">
      <!-- 1. 网关地址 -->
      <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
        <div class="flex items-center justify-between gap-2">
          <span
            class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
          >
            <Globe class="size-3.5 text-primary" aria-hidden="true" />
            网关地址
          </span>
          <CopyButton :text="endpoint" />
        </div>
        <div class="mt-2 truncate font-mono text-title font-semibold text-foreground tabular-nums">
          {{ endpoint }}
        </div>
        <div class="mt-1 text-caption text-muted-foreground">
          {{ profile.auth === 'saml' ? 'SAML SSO 单点登录' : '账号密码认证' }}
        </div>
      </div>

      <!-- 2. 登录账号 -->
      <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
        <div class="flex items-center justify-between gap-2">
          <span
            class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
          >
            <User class="size-3.5 text-primary" aria-hidden="true" />
            登录身份
          </span>
        </div>
        <div class="mt-2 truncate text-title font-semibold text-foreground">
          {{ profile.username || (profile.auth === 'saml' ? 'SSO 浏览器登录' : '—') }}
        </div>
        <div class="mt-1 truncate text-caption text-muted-foreground">
          <template v-if="profile.realm">Realm: {{ profile.realm }} · </template>
          <template v-if="profile.auth === 'saml'">通过浏览器完成授权</template>
          <template v-else>
            {{ profile.hasPassword ? '已保存登录密码' : '未保存登录密码' }}
          </template>
        </div>
      </div>

      <!-- 3. 证书校验 -->
      <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
        <div class="flex items-center justify-between gap-2">
          <span
            class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
          >
            <ShieldCheck
              v-if="profile.trustedCert"
              class="size-3.5 text-primary"
              aria-hidden="true"
            />
            <Lock v-else class="size-3.5 text-primary" aria-hidden="true" />
            证书校验
          </span>
        </div>
        <div class="mt-2 truncate text-title font-semibold text-foreground">
          {{ profile.trustedCert ? '指定证书指纹' : '系统默认校验' }}
        </div>
        <div
          class="mt-1 truncate text-caption text-muted-foreground"
          :class="profile.trustedCert ? 'font-mono' : ''"
        >
          {{
            profile.trustedCert
              ? `${profile.trustedCert.slice(0, 12)}…${profile.trustedCert.slice(-12)}`
              : '使用系统受信任根证书自动验证'
          }}
        </div>
      </div>

      <!-- 4. 路由与 DNS 策略 -->
      <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
        <div class="flex items-center justify-between gap-2">
          <span
            class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
          >
            <SlidersHorizontal class="size-3.5 text-primary" aria-hidden="true" />
            路由与 DNS
          </span>
        </div>
        <div class="mt-2 truncate text-title font-semibold text-foreground tabular-nums">
          {{ routePolicySummary }}
        </div>
        <div class="mt-1 truncate text-caption text-muted-foreground">
          {{ dnsPolicySummary }}
        </div>
      </div>
    </div>
  </div>
</template>
