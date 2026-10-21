<script setup lang="ts">
import { computed } from 'vue'
import { ArrowLeftRight, Globe, Loader2, User } from 'lucide-vue-next'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import { profileStatus } from '@/lib/status'
import { formatDurationSince } from '@/lib/format'
import StatusDot from '@/components/common/StatusDot.vue'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  profile: Profile
  vpnState: VpnState
  pending: boolean
  nowMs: number
  helperReady: boolean
}>()

const emit = defineEmits<{
  connect: [id: string]
  cancel: []
  disconnect: []
}>()

const view = computed(() => profileStatus(props.profile.id, props.vpnState))

const isBusyOnThisProfile = computed(
  () =>
    props.pending && (!props.vpnState.profileId || props.vpnState.profileId === props.profile.id),
)

const otherDisconnecting = computed(
  () => view.value.otherActive && props.vpnState.status === 'disconnecting',
)

const subtitle = computed(() => {
  const { status, label, otherActive } = view.value
  if (status === 'connected') {
    const duration = formatDurationSince(props.vpnState.connectedAt, props.nowMs)
    return duration ? `${label} · ${duration}` : label
  }
  if (otherActive) return `${label} · 「${props.vpnState.profileName || '其他连接'}」正在使用中`
  return label
})

const connectBlockedReason = computed(() => {
  if (!props.helperReady) return '特权辅助组件 tunnelkit-helper 尚未就绪'
  if (otherDisconnecting.value) return '正在断开当前活动的连接，请稍候'
  return ''
})

const badgeClass = computed(() => {
  switch (view.value.tone) {
    case 'ok':
      return 'bg-status-ok/14 text-status-ok'
    case 'busy':
      return 'bg-status-busy/16 text-status-busy'
    case 'danger':
      return 'bg-status-danger/14 text-status-danger'
    default:
      return 'bg-muted text-muted-foreground'
  }
})
</script>

<template>
  <header class="surface-card mx-6 flex items-center justify-between gap-4 rounded-2xl px-5 py-4">
    <div class="min-w-0">
      <div class="flex items-center gap-2.5">
        <h1 class="truncate text-heading font-semibold tracking-tight text-foreground">
          {{ profile.name }}
        </h1>
        <span
          class="inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-0.5 text-caption font-medium tabular-nums"
          :class="badgeClass"
        >
          <StatusDot :tone="view.tone" />
          <span>{{ subtitle }}</span>
        </span>
      </div>

      <div
        class="mt-1 flex flex-wrap items-center gap-x-3.5 gap-y-1 text-caption text-muted-foreground"
      >
        <span class="inline-flex items-center gap-1 font-mono tabular-nums">
          <Globe class="size-3.5 opacity-75" aria-hidden="true" />
          {{ profile.host }}:{{ profile.port }}
        </span>
        <span class="inline-flex items-center gap-1">
          <User class="size-3.5 opacity-75" aria-hidden="true" />
          {{ profile.username || (profile.auth === 'saml' ? 'SAML SSO' : '—') }}
        </span>
      </div>
    </div>

    <div class="app-no-drag shrink-0">
      <Button
        v-if="view.status === 'connected'"
        variant="destructive"
        class="min-w-[88px]"
        :disabled="pending"
        @click="emit('disconnect')"
      >
        断开连接
      </Button>

      <Button
        v-else-if="view.status === 'connecting'"
        variant="outline"
        class="min-w-[88px]"
        @click="emit('cancel')"
      >
        <Loader2 class="size-3.5 animate-spin" aria-hidden="true" />
        取消连接
      </Button>

      <Button
        v-else-if="view.status === 'disconnecting'"
        variant="outline"
        class="min-w-[88px]"
        disabled
      >
        <Loader2 class="size-3.5 animate-spin" aria-hidden="true" />
        正在断开…
      </Button>

      <Button
        v-else
        class="min-w-[88px]"
        :disabled="isBusyOnThisProfile || Boolean(connectBlockedReason)"
        :title="connectBlockedReason || undefined"
        @click="emit('connect', profile.id)"
      >
        <Loader2 v-if="isBusyOnThisProfile" class="size-3.5 animate-spin" aria-hidden="true" />
        <ArrowLeftRight v-else-if="view.otherActive" class="size-3.5" aria-hidden="true" />
        {{ view.otherActive ? '切换连接' : '连接' }}
      </Button>
    </div>
  </header>
</template>
