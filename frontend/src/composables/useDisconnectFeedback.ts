import { onScopeDispose, ref, watch } from 'vue'
import type { VpnPendingAction, VpnState } from '@/types/vpn'

const MINIMUM_FEEDBACK_MS = 350

/** Keep a short button animation without extending the actual VPN lifecycle. */
export function useDisconnectFeedback(source: {
  readonly profileId: string
  readonly pendingAction?: VpnPendingAction | null
  readonly state: VpnState
  readonly busy: boolean
}) {
  const visible = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  const clear = () => {
    clearTimeout(timer)
    timer = undefined
    visible.value = false
  }

  watch(
    () => [source.profileId, source.pendingAction] as const,
    ([profileId, action], previous) => {
      if (profileId !== previous?.[0] || (action && action.kind !== 'disconnect')) clear()
      if (action?.kind !== 'disconnect' || action.profileId !== profileId) return
      clear()
      visible.value = true
      timer = setTimeout(clear, MINIMUM_FEEDBACK_MS)
    },
    { flush: 'sync', immediate: true },
  )
  watch(
    () => [source.state.status, source.state.error, source.busy] as const,
    ([status, error, busy]) => {
      if (error || status === 'connecting' || (status === 'connected' && !busy)) clear()
    },
    { flush: 'sync' },
  )
  onScopeDispose(clear)
  return visible
}
