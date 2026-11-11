<script setup lang="ts">
import { computed } from 'vue'
import { Globe, Loader2, ShieldAlert, ShieldCheck, User } from '@lucide/vue'
import CopyButton from '@/components/common/CopyButton.vue'
import InlineAlert from '@/components/common/InlineAlert.vue'
import StateText from '@/components/common/StateText.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import AppButton from '@/components/base/button/AppButton.vue'
import { useDisconnectFeedback } from '@/composables/useDisconnectFeedback'
import { isTransitionStatus } from '@/domain/vpn'
import { getToneClasses } from '@/constants/tone'
import { getViewingStatusMeta } from '@/presentation/vpn'
import type { Profile } from '@/types/profile'
import type { VpnPendingAction, VpnState } from '@/types/vpn'
import { formatEndpoint } from '@/utils/network'

const props = withDefaults(
  defineProps<{
    profile: Profile
    state: VpnState
    elapsedText: string
    busy?: boolean
    pendingAction?: VpnPendingAction | null
    canceling?: boolean
  }>(),
  {
    busy: false,
    canceling: false,
  },
)

const emit = defineEmits<{
  toggleConnect: []
  trustCert: []
}>()

const isThisProfileActive = computed(() =>
  Boolean(props.state.profileId && props.state.profileId === props.profile.id),
)

const statusMeta = computed(() => getViewingStatusMeta(props.state, props.profile.id))

const subtitle = computed(() =>
  isThisProfileActive.value && props.state.status === 'connected'
    ? [statusMeta.value.label, props.elapsedText].filter(Boolean).join(' · ')
    : statusMeta.value.label,
)
const pendingAction = computed(() =>
  props.pendingAction?.profileId === props.profile.id ? props.pendingAction : null,
)
const disconnectFeedback = useDisconnectFeedback({
  get profileId() {
    return props.profile.id
  },
  get pendingAction() {
    return props.pendingAction
  },
  get state() {
    return props.state
  },
  get busy() {
    return props.busy
  },
})
const actionButtonConfig = computed(() => {
  if (
    disconnectFeedback.value ||
    (pendingAction.value?.kind === 'disconnect' && props.state.status === 'connected')
  ) {
    return { label: '正在断开…', variant: 'outline' as const, disabled: true, loading: true }
  }
  if (isThisProfileActive.value && props.state.status === 'connected') {
    return {
      label: '断开连接',
      variant: 'destructive' as const,
      disabled: props.busy,
      loading: pendingAction.value?.kind === 'connect',
    }
  }
  if (isThisProfileActive.value && props.state.status === 'connecting') {
    return {
      label: props.canceling ? '正在取消…' : '取消连接',
      variant: 'outline' as const,
      disabled: props.canceling,
      loading: true,
    }
  }
  if (props.state.status === 'disconnecting') {
    return {
      label: '正在断开…',
      variant: 'outline' as const,
      disabled: true,
      loading: true,
    }
  }
  if (props.state.status !== 'disconnected' && !isThisProfileActive.value) {
    return {
      label: '切换连接',
      variant: 'default' as const,
      disabled: props.busy || isTransitionStatus(props.state.status),
      loading: pendingAction.value?.kind === 'connect',
    }
  }
  return {
    label: '连接',
    variant: 'default' as const,
    disabled: props.busy,
    loading: pendingAction.value?.kind === 'connect',
  }
})
</script>

<template>
  <section class="surface-card mx-6 rounded-2xl px-5 py-4">
    <div class="flex min-w-0 items-center justify-between gap-4">
      <div class="min-w-0">
        <div class="flex items-center gap-2.5">
          <h1 class="truncate text-heading font-semibold tracking-tight text-foreground">
            {{ profile.name }}
          </h1>
          <span
            class="inline-flex shrink-0 transition-colors duration-200 items-center gap-1.5 rounded-full px-2.5 py-0.5 text-caption font-medium tabular-nums"
            :class="getToneClasses(statusMeta.tone).pill"
          >
            <StatusDot :tone="statusMeta.tone" :pulse="statusMeta.pulse" />
            <StateText :text="subtitle" :transition-key="statusMeta.label" />
          </span>
        </div>

        <div
          class="mt-1 flex flex-wrap items-center gap-x-3.5 gap-y-1 text-caption text-muted-foreground"
        >
          <span class="inline-flex items-center gap-1 font-mono tabular-nums">
            <Globe class="size-3.5 opacity-75" aria-hidden="true" />
            {{ formatEndpoint(profile.host, profile.port) }}
          </span>
          <span class="inline-flex items-center gap-1">
            <User class="size-3.5 opacity-75" aria-hidden="true" />
            {{ profile.username || (profile.auth === 'saml' ? 'SAML SSO' : '—') }}
          </span>
        </div>
      </div>

      <div class="app-no-drag shrink-0">
        <AppButton
          :variant="actionButtonConfig.variant"
          class="min-w-[88px] disabled:opacity-100"
          :disabled="actionButtonConfig.disabled"
          :aria-busy="actionButtonConfig.loading"
          data-testid="vpn-connect-toggle"
          @click="emit('toggleConnect')"
        >
          <Loader2
            v-if="actionButtonConfig.loading"
            class="size-3.5 animate-spin"
            aria-hidden="true"
          />
          <StateText :text="actionButtonConfig.label" />
        </AppButton>
      </div>
    </div>
    <div
      v-if="isThisProfileActive && state.status === 'disconnected' && state.error?.message"
      data-testid="vpn-error-banner"
      class="mt-4"
    >
      <InlineAlert tone="danger">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0 max-w-full space-y-1 break-words">
            <div class="flex items-center gap-1.5 font-semibold">
              <ShieldAlert class="size-4 shrink-0" />
              <span class="min-w-0 break-words">{{ state.error.message }}</span>
            </div>
            <p v-if="state.error.hint" class="pl-5 text-micro opacity-90">
              {{ state.error.hint }}
            </p>
            <div
              v-if="state.error.trustedCert"
              class="flex flex-wrap items-center gap-1.5 pl-5 font-mono text-micro opacity-85"
            >
              <span class="break-all">SHA-256: {{ state.error.trustedCert }}</span>
              <CopyButton :text="state.error.trustedCert" label="复制指纹" />
            </div>
          </div>

          <AppButton
            v-if="state.error.trustedCert"
            variant="outline"
            size="sm"
            data-testid="vpn-trust-cert-btn"
            class="shrink-0 bg-card text-foreground hover:bg-muted"
            @click="emit('trustCert')"
          >
            <ShieldCheck class="size-3.5 text-tone-success-fg" />
            信任此证书并重连
          </AppButton>
        </div>
      </InlineAlert>
    </div>
  </section>
</template>
