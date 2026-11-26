import { computed, onScopeDispose, ref } from 'vue'
import { defineStore } from 'pinia'
import { systemApi } from '@/api/system'
import { GITHUB_RELEASES_URL, GITHUB_REPO_URL } from '@/constants/releases'
import { DISMISSED_UPDATE_VERSION_KEY } from '@/constants/storage'
import {
  formatErrorMessage,
  isElevationCancelledError,
  isUpdateCancelledError,
  isUpdateStagedInvalidError,
} from '@/presentation/error'
import { useNoticeStore } from '@/stores/useNoticeStore'
import type { SystemInfo, UpdateInfo, UpdatePhase, UpdateProgress } from '@/types/system'

const UP_TO_DATE_FEEDBACK_MS = 2500

function readDismissedVersion(): string {
  try {
    return globalThis.localStorage?.getItem(DISMISSED_UPDATE_VERSION_KEY) ?? ''
  } catch {
    return ''
  }
}

export const useSystemStore = defineStore('system', () => {
  const noticeStore = useNoticeStore()

  const systemInfo = ref<SystemInfo | null>(null)
  const updateInfo = ref<UpdateInfo | null>(null)
  const updateProgress = ref<UpdateProgress | null>(null)
  const updateDialogOpen = ref(false)
  const checkingSystem = ref(false)
  const isCheckingUpdate = ref(false)
  const isDownloadingUpdate = ref(false)
  const isCancelingUpdate = ref(false)
  const isInstallingUpdate = ref(false)
  const readyVersion = ref('')
  const isOpeningRelease = ref(false)
  const updateUpToDate = ref(false)
  const hasCheckedUpdateOnce = ref(false)
  const dismissedVersion = ref<string>(readDismissedVersion())

  let checkInFlight: Promise<UpdateInfo | null> | null = null
  let explicitCheckPending = false
  let inlineFeedbackPending = false
  let stopProgressSub: (() => void) | null = null
  let upToDateTimer: ReturnType<typeof setTimeout> | undefined

  const clearUpToDateFeedback = () => {
    clearTimeout(upToDateTimer)
    upToDateTimer = undefined
    updateUpToDate.value = false
  }

  const markUpToDateFeedback = () => {
    clearUpToDateFeedback()
    updateUpToDate.value = true
    upToDateTimer = setTimeout(() => {
      upToDateTimer = undefined
      updateUpToDate.value = false
    }, UP_TO_DATE_FEEDBACK_MS)
  }

  onScopeDispose(clearUpToDateFeedback)

  const appVersion = computed(
    () => systemInfo.value?.appVersion || updateInfo.value?.currentVersion || '',
  )

  const hasUpdate = computed(() => Boolean(updateInfo.value?.hasUpdate))

  const isUpdateReady = computed(() =>
    Boolean(
      readyVersion.value &&
      updateInfo.value?.latestVersion &&
      readyVersion.value === updateInfo.value.latestVersion &&
      !isDownloadingUpdate.value &&
      !isInstallingUpdate.value,
    ),
  )

  const isApplyingUpdate = computed(() => isDownloadingUpdate.value || isInstallingUpdate.value)

  const updatePhase = computed<UpdatePhase>(() => {
    if (isInstallingUpdate.value) return 'installing'
    if (isDownloadingUpdate.value) {
      return updateProgress.value?.stage === 'verifying' ? 'verifying' : 'downloading'
    }
    if (isUpdateReady.value) return 'ready'
    return 'available'
  })

  const buildReadyProgress = (bytes?: number): UpdateProgress => {
    const size = Math.max(
      bytes ?? 0,
      updateProgress.value?.totalBytes ?? 0,
      updateProgress.value?.downloadedBytes ?? 0,
      updateInfo.value?.assetSize ?? 0,
    )
    return {
      stage: 'ready',
      downloadedBytes: size,
      totalBytes: size,
      percent: 100,
      message: '可以开始安装了',
    }
  }

  const loadSystemInfo = async (): Promise<SystemInfo | null> => {
    checkingSystem.value = true
    try {
      const info = await systemApi.getInfo()
      systemInfo.value = info
      return info
    } catch {
      return null
    } finally {
      checkingSystem.value = false
    }
  }

  const startWatching = () => {
    stopProgressSub ??= systemApi.subscribeUpdateProgress((progress) => {
      if (!isDownloadingUpdate.value && !isInstallingUpdate.value) return
      if (
        isDownloadingUpdate.value &&
        isCancelingUpdate.value &&
        progress.stage === 'downloading'
      ) {
        return
      }
      updateProgress.value = progress
      if (progress.stage === 'ready' && updateInfo.value?.latestVersion) {
        readyVersion.value = updateInfo.value.latestVersion
      }
    })
  }

  const stopWatching = () => {
    stopProgressSub?.()
    stopProgressSub = null
    clearUpToDateFeedback()
  }

  const checkUpdate = async (
    options: { silent?: boolean; inline?: boolean } = {},
  ): Promise<UpdateInfo | null> => {
    const requestedSilent = Boolean(options.silent)
    const requestedInline = Boolean(options.inline)
    if (requestedSilent && (hasCheckedUpdateOnce.value || checkInFlight !== null)) {
      return checkInFlight ?? updateInfo.value
    }

    if (!requestedSilent) {
      explicitCheckPending = true
      if (requestedInline) inlineFeedbackPending = true
      clearUpToDateFeedback()
    }
    if (checkInFlight) {
      return checkInFlight
    }

    hasCheckedUpdateOnce.value = true
    isCheckingUpdate.value = true

    const task = (async (): Promise<UpdateInfo | null> => {
      try {
        const info = await systemApi.checkUpdate()
        const silent = !explicitCheckPending
        const inline = inlineFeedbackPending
        updateInfo.value = info
        if (!info.hasUpdate || (readyVersion.value && readyVersion.value !== info.latestVersion)) {
          readyVersion.value = ''
          if (!isApplyingUpdate.value) {
            updateProgress.value = null
          }
        }
        if (info.hasUpdate) {
          clearUpToDateFeedback()
          const isDismissed =
            Boolean(dismissedVersion.value) && dismissedVersion.value === info.latestVersion
          if (!silent || !isDismissed) {
            updateDialogOpen.value = true
          }
        } else if (!silent && inline) {
          markUpToDateFeedback()
        } else if (!silent) {
          noticeStore.showSuccess(
            '当前已是最新版本',
            info.currentVersion ? `v${info.currentVersion}` : undefined,
          )
        }
        return info
      } catch (err) {
        const silent = !explicitCheckPending
        clearUpToDateFeedback()
        if (!silent) {
          noticeStore.showError(err, '检查更新失败')
        }
        return null
      } finally {
        checkInFlight = null
        explicitCheckPending = false
        inlineFeedbackPending = false
        isCheckingUpdate.value = false
      }
    })()

    checkInFlight = task
    return task
  }

  const downloadUpdate = async () => {
    if (isDownloadingUpdate.value || isInstallingUpdate.value) return
    if (isUpdateReady.value) {
      updateDialogOpen.value = true
      return
    }
    startWatching()
    isDownloadingUpdate.value = true
    isCancelingUpdate.value = false
    updateProgress.value = {
      stage: 'downloading',
      downloadedBytes: 0,
      totalBytes: updateInfo.value?.assetSize ?? 0,
      percent: 0,
      message: '正在下载更新…',
    }
    try {
      await systemApi.downloadUpdate()
      const version = updateInfo.value?.latestVersion ?? ''
      readyVersion.value = version
      updateProgress.value = buildReadyProgress()
      updateDialogOpen.value = true
    } catch (err) {
      updateProgress.value = null
      if (!isUpdateCancelledError(err)) {
        noticeStore.showError(err, '下载更新失败')
      }
    } finally {
      isDownloadingUpdate.value = false
      isCancelingUpdate.value = false
    }
  }

  const cancelUpdate = async () => {
    if (
      !isDownloadingUpdate.value ||
      updatePhase.value !== 'downloading' ||
      isCancelingUpdate.value
    ) {
      return
    }
    isCancelingUpdate.value = true
    try {
      const cancelled = await systemApi.cancelUpdate()
      if (!cancelled) {
        isCancelingUpdate.value = false
      }
    } catch {
      isCancelingUpdate.value = false
    }
  }

  const installUpdate = async () => {
    if (isInstallingUpdate.value || isDownloadingUpdate.value) return
    startWatching()
    const readyBytes = Math.max(
      updateProgress.value?.totalBytes ?? 0,
      updateProgress.value?.downloadedBytes ?? 0,
      updateInfo.value?.assetSize ?? 0,
    )
    isInstallingUpdate.value = true
    updateProgress.value = {
      stage: 'installing',
      downloadedBytes: readyBytes,
      totalBytes: readyBytes,
      percent: 100,
      message: '正在安装更新…',
    }
    try {
      await systemApi.installUpdate()
    } catch (err) {
      isInstallingUpdate.value = false
      if (isUpdateStagedInvalidError(err)) {
        readyVersion.value = ''
        updateProgress.value = null
        noticeStore.showError(err, '安装更新失败')
      } else {
        updateProgress.value = readyVersion.value ? buildReadyProgress(readyBytes) : null
        if (isElevationCancelledError(err)) {
          const detail = formatErrorMessage(err, '已取消管理员授权')
          noticeStore.showWarning(
            '已取消管理员授权',
            detail === '已取消管理员授权'
              ? undefined
              : detail.replace(/^已取消管理员授权[，,:：\s]*/, '') || undefined,
          )
        } else {
          noticeStore.showError(err, '安装更新失败')
        }
      }
    }
  }

  const deferInstall = () => {
    if (isApplyingUpdate.value) return
    updateDialogOpen.value = false
  }

  const dismissUpdate = (version?: string) => {
    if (isApplyingUpdate.value) return
    const target = (version || updateInfo.value?.latestVersion || '').trim()
    if (target) {
      dismissedVersion.value = target
      try {
        globalThis.localStorage?.setItem(DISMISSED_UPDATE_VERSION_KEY, target)
      } catch {
        // 忽略受限环境下的 localStorage 异常
      }
    }
    updateDialogOpen.value = false
  }

  const showUpdateDialog = () => {
    updateDialogOpen.value = true
  }

  const openReleasePage = async (url?: string) => {
    if (isOpeningRelease.value) return
    const targetUrl = url || updateInfo.value?.releaseUrl || GITHUB_RELEASES_URL
    isOpeningRelease.value = true
    try {
      await systemApi.openExternalUrl(targetUrl)
      if (!isApplyingUpdate.value) {
        updateDialogOpen.value = false
      }
    } catch (err) {
      noticeStore.showError(err, '打开版本发布页面失败')
    } finally {
      isOpeningRelease.value = false
    }
  }

  const openRepository = async () => {
    try {
      await systemApi.openExternalUrl(GITHUB_REPO_URL)
    } catch (err) {
      noticeStore.showError(err, '打开开源仓库页面失败')
    }
  }

  return {
    systemInfo,
    updateInfo,
    updateProgress,
    updateDialogOpen,
    checkingSystem,
    isCheckingUpdate,
    isDownloadingUpdate,
    isCancelingUpdate,
    isInstallingUpdate,
    isUpdateReady,
    isApplyingUpdate,
    updatePhase,
    isOpeningRelease,
    updateUpToDate,
    appVersion,
    hasUpdate,
    loadSystemInfo,
    startWatching,
    stopWatching,
    clearUpToDateFeedback,
    checkUpdate,
    downloadUpdate,
    cancelUpdate,
    installUpdate,
    deferInstall,
    dismissUpdate,
    showUpdateDialog,
    openReleasePage,
    openRepository,
  }
})
