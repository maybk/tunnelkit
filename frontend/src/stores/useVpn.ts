import { ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import { Events } from '@wailsio/runtime'
import {
  VPN_STATE_EVENT,
  answerVpnPrompt,
  cancelConnect,
  clearVpnLog,
  connectVpn,
  disconnectVpn,
  getVpnLog,
  getVpnState,
  toVpnState,
  type VpnLogView,
  type VpnState,
} from '@/api/vpn'
import { useNotice } from '@/stores/useNotice'
import type * as vpnModels from '../../bindings/tunnelkit/internal/vpn/models'

export const useVpn = createGlobalState(() => {
  const { showError } = useNotice()

  const state = ref<VpnState>({ status: 'disconnected', phase: 'disconnected' })
  const pending = ref(false)
  const answering = ref(false)
  const logView = ref<VpnLogView | null>(null)
  const logLoading = ref(false)
  const nowMs = ref(Date.now())

  let clockTimer: ReturnType<typeof setInterval> | null = null
  let unsubscribe: (() => void) | null = null

  const loadState = async () => {
    state.value = await getVpnState()
  }

  const startWatching = () => {
    unsubscribe ??= Events.On(VPN_STATE_EVENT, (event) => {
      state.value = toVpnState(event.data as vpnModels.State)
    })
    clockTimer ??= setInterval(() => {
      nowMs.value = Date.now()
    }, 1000)
  }

  const stopWatching = () => {
    unsubscribe?.()
    unsubscribe = null
    if (clockTimer) clearInterval(clockTimer)
    clockTimer = null
  }

  let activeOps = 0
  let pendingProfileId = ''

  // 后端会把大多数失败写入 state.error 并推送；只有未反映到状态里的错误（如操作被拒绝）才弹出全局提示。
  const run = async (action: () => Promise<void>, targetProfileId = '') => {
    if (pending.value) {
      const activeId = pendingProfileId || state.value.profileId || ''
      const isSwitchingProfile = Boolean(
        targetProfileId && activeId && targetProfileId !== activeId,
      )
      if (!isSwitchingProfile) return
    }
    activeOps++
    pending.value = true
    if (targetProfileId) pendingProfileId = targetProfileId
    try {
      await action()
    } catch (err) {
      await loadState().catch(() => {})
      if (!state.value.error) showError(err)
    } finally {
      activeOps--
      if (activeOps <= 0) {
        activeOps = 0
        pending.value = false
        pendingProfileId = ''
      }
    }
  }

  const connect = (profileId: string) => run(() => connectVpn(profileId), profileId)
  const disconnect = () => run(disconnectVpn)

  const cancel = async () => {
    try {
      await cancelConnect()
    } catch (err) {
      showError(err)
    }
  }

  const answerPrompt = async (promptId: string, value: string, cancelPrompt = false) => {
    if (answering.value) return
    answering.value = true
    try {
      await answerVpnPrompt(promptId, value, cancelPrompt)
    } catch (err) {
      showError(err)
    } finally {
      answering.value = false
    }
  }

  const withLog = async (action: () => Promise<void>) => {
    logLoading.value = true
    try {
      await action()
    } catch (err) {
      showError(err)
    } finally {
      logLoading.value = false
    }
  }

  const loadLog = (profileId: string) =>
    withLog(async () => {
      logView.value = await getVpnLog(profileId)
    })

  const clearLog = (profileId: string) =>
    withLog(async () => {
      await clearVpnLog(profileId)
      logView.value = await getVpnLog(profileId)
    })

  return {
    state,
    pending,
    answering,
    logView,
    logLoading,
    nowMs,
    loadState,
    startWatching,
    stopWatching,
    connect,
    cancel,
    answerPrompt,
    disconnect,
    loadLog,
    clearLog,
  }
})
