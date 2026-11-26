<script setup lang="ts">
import { computed, type Component } from 'vue'
import {
  ArrowLeftRight,
  CheckCircle2,
  Globe,
  Loader2,
  Power,
  ShieldAlert,
  ShieldCheck,
  User,
} from '@lucide/vue'
import AppButton from '@/components/base/button/AppButton.vue'
import AppCard from '@/components/base/card/AppCard.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import InlineAlert from '@/components/common/InlineAlert.vue'
import StateText from '@/components/common/StateText.vue'
import { useDisconnectFeedback } from '@/composables/useDisconnectFeedback'
import { getToneClasses } from '@/constants/tone'
import { isTransitionStatus } from '@/domain/vpn'
import { getViewingStatusMeta } from '@/presentation/vpn'
import type { Profile } from '@/types/profile'
import type { VpnPendingAction, VpnState } from '@/types/vpn'
import { formatEndpoint } from '@/utils/network'

type HeroIconKind = 'connected' | 'loading' | 'error' | 'idle'

const HERO_ICONS: Record<HeroIconKind, Component> = {
  connected: CheckCircle2,
  loading: Loader2,
  error: ShieldAlert,
  idle: Power,
}

const props = withDefaults(
  defineProps<{
    profile: Profile
    state: VpnState
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

const heroIconKind = computed<HeroIconKind>(() => {
  if (isThisProfileActive.value && props.state.status === 'connected') return 'connected'
  if (
    isThisProfileActive.value &&
    (props.state.status === 'connecting' || props.state.status === 'disconnecting')
  ) {
    return 'loading'
  }
  if (statusMeta.value.tone === 'danger') return 'error'
  return 'idle'
})

const heroIconBoxClass = computed(() => {
  const tone = statusMeta.value.tone === 'neutral' ? 'brand' : statusMeta.value.tone
  return getToneClasses(tone).iconBox
})

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
    return {
      label: '正在断开…',
      variant: 'outline' as const,
      disabled: true,
      loading: true,
      icon: Loader2,
    }
  }
  if (isThisProfileActive.value && props.state.status === 'connected') {
    const loading = pendingAction.value?.kind === 'connect'
    return {
      label: '断开连接',
      variant: 'destructive' as const,
      disabled: props.busy,
      loading,
      icon: loading ? Loader2 : Power,
    }
  }
  if (isThisProfileActive.value && props.state.status === 'connecting') {
    return {
      label: props.canceling ? '正在取消…' : '取消连接',
      variant: 'outline' as const,
      disabled: props.canceling,
      loading: true,
      icon: Loader2,
    }
  }
  if (props.state.status === 'disconnecting') {
    return {
      label: '正在断开…',
      variant: 'outline' as const,
      disabled: true,
      loading: true,
      icon: Loader2,
    }
  }
  if (props.state.status !== 'disconnected' && !isThisProfileActive.value) {
    const loading = pendingAction.value?.kind === 'connect'
    return {
      label: '切换连接',
      variant: 'default' as const,
      disabled: props.busy || isTransitionStatus(props.state.status),
      loading,
      icon: loading ? Loader2 : ArrowLeftRight,
    }
  }
  const loading = pendingAction.value?.kind === 'connect'
  return {
    label: '连接',
    variant: 'default' as const,
    disabled: props.busy,
    loading,
    icon: loading ? Loader2 : Power,
  }
})
</script>

<template>
  <AppCard
    as="section"
    padding="none"
    class="mx-6 px-5 py-4 transition-[box-shadow,background-color,border-color] duration-200 ease-(--ease-out-kit)"
  >
    <div class="flex min-w-0 items-center justify-between gap-4">
      <div class="flex min-w-0 items-center gap-4">
        <div
          data-testid="hero-status-icon"
          :data-state="heroIconKind"
          :class="[
            'relative flex size-12 shrink-0 items-center justify-center rounded-xl border transition-[color,background-color,border-color] duration-200 ease-(--ease-out-kit)',
            heroIconBoxClass,
          ]"
        >
          <Transition
            enter-active-class="transition-[opacity,scale,filter] duration-200 ease-[cubic-bezier(0.2,0,0,1)]"
            enter-from-class="scale-50 opacity-0 blur-[3px]"
            leave-active-class="pointer-events-none absolute transition-[opacity,scale,filter] duration-150 ease-[cubic-bezier(0.2,0,0,1)]"
            leave-to-class="scale-50 opacity-0 blur-[3px]"
          >
            <component
              :is="HERO_ICONS[heroIconKind]"
              :key="heroIconKind"
              :class="[
                'size-6',
                heroIconKind === 'connected' && 'stroke-[2.2]',
                heroIconKind === 'loading' && 'motion-safe:animate-spin',
              ]"
              aria-hidden="true"
            />
          </Transition>
        </div>

        <div class="min-w-0 space-y-0.5">
          <h2 data-testid="hero-status-title" class="truncate text-title font-bold text-foreground">
            <StateText :text="statusMeta.label" :transition-key="statusMeta.label" />
          </h2>

          <div
            class="flex flex-wrap items-center gap-x-3.5 gap-y-0.5 text-caption text-muted-foreground"
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
      </div>

      <div class="app-no-drag shrink-0">
        <AppButton
          :variant="actionButtonConfig.variant"
          class="min-w-26 pr-5 pl-4.5 font-semibold disabled:opacity-100"
          :disabled="actionButtonConfig.disabled"
          :aria-busy="actionButtonConfig.loading"
          data-testid="vpn-connect-toggle"
          @click="emit('toggleConnect')"
        >
          <component
            :is="actionButtonConfig.icon"
            :class="['size-4', actionButtonConfig.loading && 'animate-spin']"
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
  </AppCard>
</template>
