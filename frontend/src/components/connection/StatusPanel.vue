<script setup lang="ts">
import { computed } from 'vue'
import {
  Activity,
  AlertTriangle,
  ArrowDownUp,
  Globe,
  Lock,
  Network,
  ShieldCheck,
  SlidersHorizontal,
  User,
} from 'lucide-vue-next'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import { profileStatus } from '@/lib/status'
import { formatBytes, formatClockTime, formatDurationSince } from '@/lib/format'
import CopyButton from '@/components/common/CopyButton.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import ErrorCard from './ErrorCard.vue'

const props = defineProps<{
  profile: Profile
  vpnState: VpnState
  pending: boolean
  nowMs: number
}>()

const emit = defineEmits<{
  trustAndReconnect: [sha256: string]
}>()

const view = computed(() => profileStatus(props.profile.id, props.vpnState))
const endpoint = computed(() => `${props.profile.host}:${props.profile.port}`)

const sessionDuration = computed(() => {
  if (view.value.status !== 'connected' || !props.vpnState.connectedAt) return ''
  return formatDurationSince(props.vpnState.connectedAt, props.nowMs)
})

const liveInfo = computed(() =>
  view.value.status === 'connected' ? (props.vpnState.info ?? null) : null,
)
const liveStats = computed(() =>
  view.value.status === 'connected' ? (props.vpnState.stats ?? null) : null,
)

const statusSummary = computed(() => {
  switch (view.value.status) {
    case 'connected':
      return {
        title: '已建立安全连接',
        detail: props.vpnState.connectedAt
          ? `连接建立于 ${formatClockTime(props.vpnState.connectedAt)}，网络流量正通过当前 VPN 转发`
          : '网络流量正通过当前 VPN 转发',
      }
    case 'connecting':
      return {
        title: view.value.label || '正在连接服务器…',
        detail: '若弹出系统权限或验证码窗口，请按提示完成确认',
      }
    case 'disconnecting':
      return {
        title: '正在断开连接…',
        detail: '正在关闭隧道并恢复原有路由与 DNS 设置',
      }
    default:
      if (view.value.error) {
        return {
          title: '上次连接未成功',
          detail: '请根据上方提示检查配置后重新连接',
        }
      }
      if (view.value.otherActive) {
        const activeName = props.vpnState.profileName || '其他连接'
        return {
          title: '当前未连接',
          detail: `当前正连接至「${activeName}」，点击右上角「切换连接」将自动断开并接入该网络`,
        }
      }
      return {
        title: '当前未连接',
        detail: '点击右上角「连接」按钮即可接入该网络',
      }
  }
})

const routePolicySummary = computed(() => {
  switch (props.profile.routes) {
    case 'none':
      return '仅建立连接（不改路由）'
    case 'full':
      return '全局代理（所有流量）'
    default:
      return '智能分流（仅内网流量）'
  }
})

const dnsPolicySummary = computed(() => {
  const parts: string[] = []
  const domains = props.profile.dnsDomains ?? []
  const extraRoutes = props.profile.extraRoutes ?? []

  if (props.profile.dns === 'off') {
    parts.push('保持本机默认 DNS')
  } else if (props.profile.dns === 'full') {
    parts.push('全局使用 VPN DNS')
  } else {
    parts.push(domains.length > 0 ? `智能分流 DNS（${domains.length} 个域名）` : '智能分流 DNS')
  }

  if (extraRoutes.length > 0) {
    parts.push(`附加路由 ${extraRoutes.length} 条`)
  }
  return parts.join(' · ')
})
</script>

<template>
  <div class="space-y-3.5">
    <ErrorCard
      v-if="view.error"
      :error="view.error"
      :busy="pending"
      @trust-and-reconnect="emit('trustAndReconnect', $event)"
    />

    <div
      v-if="liveInfo?.splitDnsNeedsDomains"
      class="flex items-start gap-2.5 rounded-2xl border border-status-busy/35 bg-status-busy/[0.08] px-4 py-3 text-caption text-foreground"
    >
      <AlertTriangle class="mt-0.5 size-4 shrink-0 text-status-busy" aria-hidden="true" />
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
          <StatusDot :tone="view.tone" />
          <span class="text-body font-semibold text-foreground">{{ statusSummary.title }}</span>
        </div>
        <p class="mt-1 text-caption text-muted-foreground">
          {{ statusSummary.detail }}
        </p>
      </div>

      <div v-if="view.status === 'connected' && sessionDuration" class="shrink-0 text-right">
        <div class="text-caption text-muted-foreground">持续时长</div>
        <div class="mt-0.5 font-mono text-title font-semibold text-status-ok tabular-nums">
          {{ sessionDuration }}
        </div>
      </div>
    </div>

    <!-- 已连接实时隧道参数与流量 -->
    <div v-if="liveInfo" class="grid grid-cols-2 gap-3">
      <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
        <div class="flex items-center justify-between gap-2">
          <span
            class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
          >
            <Network class="size-3.5 text-status-ok" aria-hidden="true" />
            内网 IP ({{ liveInfo.interface || 'tun' }})
          </span>
          <CopyButton v-if="liveInfo.localIp" :text="liveInfo.localIp" />
        </div>
        <div class="mt-2 truncate font-mono text-title font-semibold text-foreground tabular-nums">
          {{ liveInfo.localIp || '—' }}
        </div>
        <div class="mt-1 truncate text-caption text-muted-foreground tabular-nums">
          对端 {{ liveInfo.peerIp || '—' }} · MTU {{ liveInfo.mtu || 1354 }}
        </div>
      </div>

      <div class="surface-card flex flex-col justify-between rounded-2xl p-4">
        <div class="flex items-center justify-between gap-2">
          <span
            class="inline-flex items-center gap-1.5 text-caption font-medium text-muted-foreground"
          >
            <ArrowDownUp class="size-3.5 text-status-ok" aria-hidden="true" />
            实时流量
          </span>
          <Activity class="size-3.5 text-muted-foreground/70" aria-hidden="true" />
        </div>
        <div class="mt-2 truncate font-mono text-title font-semibold text-foreground tabular-nums">
          ↓ {{ formatBytes(liveStats?.rxBytes ?? 0) }} · ↑
          {{ formatBytes(liveStats?.txBytes ?? 0) }}
        </div>
        <div class="mt-1 truncate text-caption text-muted-foreground tabular-nums">
          路由 {{ liveInfo.routeCount || liveInfo.routes?.length || 0 }} 条 · DNS
          {{ liveInfo.dnsServers?.join(', ') || '默认' }}
        </div>
      </div>
    </div>

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
