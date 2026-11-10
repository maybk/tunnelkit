import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { vpnApi } from '@/api/vpn'
import {
  createInitialVpnState,
  isTransitionStatus,
  needsSwitchConfirm,
  resolveViewingStatus,
} from '@/domain/vpn'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { formatErrorMessage } from '@/presentation/error'
import type { VpnPendingAction, VpnState } from '@/types/vpn'
import { formatDurationSince } from '@/utils/time'

export const useVpnStore = defineStore('vpn', () => {
  const profilesStore = useProfilesStore()
  const noticeStore = useNoticeStore()

  const state = ref<VpnState>(createInitialVpnState())
  const nowMs = ref<number>(Date.now())
  const busyAction = ref(false)
  const pendingAction = ref<VpnPendingAction | null>(null)
  const cancelingConnect = ref(false)
  const answeringPrompt = ref(false)
  const promptError = ref('')
  const switchConfirmOpen = ref(false)
  const pendingSwitchProfileId = ref('')

  let stopStateSub: (() => void) | null = null
  let clockTimer: ReturnType<typeof setInterval> | null = null

  const syncClock = () => {
    if (state.value.status === 'connected' && state.value.connectedAt) {
      nowMs.value = Date.now()
      clockTimer ??= setInterval(() => {
        nowMs.value = Date.now()
      }, 1000)
    } else if (clockTimer) {
      clearInterval(clockTimer)
      clockTimer = null
    }
  }

  const applyState = (next: VpnState) => {
    const prevProfileId = state.value.profileId
    if (next.prompt?.id !== state.value.prompt?.id) promptError.value = ''
    const action = pendingAction.value
    if (
      action &&
      next.profileId === action.profileId &&
      ((action.kind === 'disconnect' && next.status !== 'connected') ||
        (action.kind === 'connect' &&
          (next.status === 'connecting' || next.status === 'connected' || next.error)))
    )
      pendingAction.value = null
    state.value = next
    syncClock()
    if (next.profileId && next.profileId !== prevProfileId && !profilesStore.selectedId) {
      profilesStore.selectProfile(next.profileId)
    }
  }

  const viewingStatus = computed(() => resolveViewingStatus(state.value, profilesStore.selectedId))

  const isViewingActiveProfile = computed(() =>
    Boolean(
      profilesStore.selectedId &&
      state.value.profileId &&
      profilesStore.selectedId === state.value.profileId,
    ),
  )

  const elapsedText = computed(() =>
    state.value.status === 'connected'
      ? formatDurationSince(state.value.connectedAt, nowMs.value)
      : '00:00:00',
  )

  const pendingSwitchProfileName = computed(() => {
    if (!pendingSwitchProfileId.value) return ''
    return (
      profilesStore.profiles.find((p) => p.id === pendingSwitchProfileId.value)?.name ?? '新连接'
    )
  })

  const loadState = async (): Promise<VpnState> => {
    const latest = await vpnApi.getState()
    applyState(latest)
    return latest
  }

  const startWatching = () => {
    stopStateSub ??= vpnApi.subscribeState((next) => {
      applyState(next)
    })
  }

  const stopWatching = () => {
    stopStateSub?.()
    stopStateSub = null
    if (clockTimer) {
      clearInterval(clockTimer)
      clockTimer = null
    }
  }

  const executeConnect = async (profileId: string) => {
    if (!profileId || busyAction.value) return
    busyAction.value = true
    pendingAction.value = { kind: 'connect', profileId }
    try {
      await vpnApi.connect(profileId)
    } catch (err) {
      noticeStore.showError(err, '发起 VPN 连接失败')
    } finally {
      pendingAction.value = null
      busyAction.value = false
    }
  }

  const requestConnect = async (profileId: string) => {
    if (!profileId) return
    if (needsSwitchConfirm(state.value, profileId)) {
      pendingSwitchProfileId.value = profileId
      switchConfirmOpen.value = true
      return
    }
    await executeConnect(profileId)
  }

  const confirmSwitchConnect = async () => {
    const targetId = pendingSwitchProfileId.value
    switchConfirmOpen.value = false
    pendingSwitchProfileId.value = ''
    if (!targetId) return
    await executeConnect(targetId)
  }

  const cancelSwitchConnect = () => {
    switchConfirmOpen.value = false
    pendingSwitchProfileId.value = ''
  }

  const disconnect = async () => {
    if (busyAction.value) return
    busyAction.value = true
    pendingAction.value = { kind: 'disconnect', profileId: state.value.profileId }
    try {
      await vpnApi.disconnect()
    } catch (err) {
      noticeStore.showError(err, '断开 VPN 连接失败')
    } finally {
      pendingAction.value = null
      busyAction.value = false
    }
  }

  const cancel = async () => {
    if (cancelingConnect.value) return
    cancelingConnect.value = true
    try {
      await vpnApi.cancel()
    } catch (err) {
      noticeStore.showError(err, '取消连接失败')
    } finally {
      cancelingConnect.value = false
    }
  }

  const toggleProfileConnection = async (profileId: string) => {
    if (!profileId) return
    const isTargetActive = state.value.profileId === profileId
    if (isTargetActive && state.value.status === 'connected') {
      await disconnect()
      return
    }
    if (isTargetActive && state.value.status === 'connecting') {
      await cancel()
      return
    }
    if (isTransitionStatus(state.value.status)) return
    await requestConnect(profileId)
  }

  const answerChallenge = async (value: string, cancelPrompt: boolean = false) => {
    const promptId = state.value.prompt?.id
    if (cancelPrompt) {
      await cancel()
      return
    }
    if (!promptId || answeringPrompt.value) return
    answeringPrompt.value = true
    promptError.value = ''
    try {
      await vpnApi.answerPrompt(promptId, value, cancelPrompt)
    } catch (err) {
      if (state.value.prompt?.id === promptId) {
        promptError.value = formatErrorMessage(err, '提交身份验证失败')
      }
    } finally {
      answeringPrompt.value = false
    }
  }

  const openSamlPrompt = async (url: string) => {
    try {
      await vpnApi.openAuthenticationUrl(url)
    } catch (err) {
      noticeStore.showError(err, '打开单点登录页面失败')
    }
  }

  const trustErrorCertAndReconnect = async () => {
    const cert = state.value.error?.trustedCert
    const profileId = state.value.profileId || profilesStore.selectedId
    if (!cert || !profileId) return
    try {
      await profilesStore.trustCert(profileId, cert)
      noticeStore.showSuccess('已信任网关证书指纹', '正在重新发起连接…')
      await executeConnect(profileId)
    } catch (err) {
      noticeStore.showError(err, '信任网关证书失败')
    }
  }

  return {
    state,
    busyAction,
    pendingAction,
    cancelingConnect,
    answeringPrompt,
    promptError,
    switchConfirmOpen,
    pendingSwitchProfileId,
    pendingSwitchProfileName,
    viewingStatus,
    isViewingActiveProfile,
    elapsedText,
    loadState,
    startWatching,
    stopWatching,
    requestConnect,
    confirmSwitchConnect,
    cancelSwitchConnect,
    disconnect,
    cancel,
    toggleProfileConnection,
    answerChallenge,
    openSamlPrompt,
    trustErrorCertAndReconnect,
  }
})
