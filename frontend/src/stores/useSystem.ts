import { computed, ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import {
  applySystemUpdate,
  checkSystemUpdate,
  getSystemInfo,
  openGitHubRepo,
  openReleaseUrl,
  subscribeUpdateProgress,
  type SystemInfo,
  type UpdateInfo,
  type UpdateProgress,
} from '@/api/system'

const DISMISSED_UPDATE_VERSION_KEY = 'tunnelkit:dismissed-update-version'

const readDismissedVersion = (): string => {
  try {
    return globalThis.localStorage?.getItem(DISMISSED_UPDATE_VERSION_KEY) ?? ''
  } catch {
    return ''
  }
}

const writeDismissedVersion = (version: string) => {
  try {
    globalThis.localStorage?.setItem(DISMISSED_UPDATE_VERSION_KEY, version)
  } catch {
    // 忽略无存储权限环境
  }
}

export const useSystem = createGlobalState(() => {
  const systemInfo = ref<SystemInfo | null>(null)
  const checking = ref(false)

  const updateInfo = ref<UpdateInfo | null>(null)
  const checkingUpdate = ref(false)
  const applyingUpdate = ref(false)
  const updateProgress = ref<UpdateProgress | null>(null)
  const updateDialogOpen = ref(false)
  const updateFeedback = ref('')

  let autoCheckStarted = false
  let unsubscribeProgress: (() => void) | null = null

  const hasUpdate = computed(() => Boolean(updateInfo.value?.hasUpdate))

  const startWatching = () => {
    unsubscribeProgress ??= subscribeUpdateProgress((progress) => {
      updateProgress.value = progress
    })
  }

  const stopWatching = () => {
    unsubscribeProgress?.()
    unsubscribeProgress = null
  }

  const refreshSystem = async () => {
    checking.value = true
    try {
      systemInfo.value = await getSystemInfo()
    } finally {
      checking.value = false
    }
  }

  const checkUpdate = async (options?: { silent?: boolean }): Promise<UpdateInfo | null> => {
    const silent = Boolean(options?.silent)
    if (silent) {
      if (autoCheckStarted || checkingUpdate.value) return updateInfo.value
      autoCheckStarted = true
    } else {
      if (checkingUpdate.value) return updateInfo.value
      autoCheckStarted = true
      checkingUpdate.value = true
      updateFeedback.value = ''
    }

    try {
      const info = await checkSystemUpdate()
      updateInfo.value = info
      if (info.hasUpdate) {
        if (!silent || info.latestVersion !== readDismissedVersion()) {
          updateDialogOpen.value = true
        }
      } else if (!silent) {
        updateFeedback.value = '当前已是最新版本'
      }
      return info
    } catch (err) {
      if (!silent) throw err
      return null
    } finally {
      if (!silent) checkingUpdate.value = false
    }
  }

  const applyUpdate = async () => {
    if (applyingUpdate.value) return
    startWatching()
    applyingUpdate.value = true
    updateProgress.value = {
      stage: 'downloading',
      downloadedBytes: 0,
      totalBytes: updateInfo.value?.assetSize ?? 0,
      percent: 0,
      message: '正在准备下载更新…',
    }
    try {
      await applySystemUpdate()
    } catch (err) {
      updateProgress.value = null
      throw err
    } finally {
      applyingUpdate.value = false
    }
  }

  const dismissUpdate = (version?: string) => {
    if (applyingUpdate.value) return
    const target = (version || updateInfo.value?.latestVersion || '').trim()
    if (target) {
      writeDismissedVersion(target)
    }
    updateDialogOpen.value = false
  }

  const openUpdateRelease = async (url?: string) => {
    const target = (url || updateInfo.value?.releaseUrl || '').trim()
    if (!target) return
    await openReleaseUrl(target)
    if (!applyingUpdate.value) {
      updateDialogOpen.value = false
    }
  }

  const openRepository = async () => {
    await openGitHubRepo()
  }

  const clearUpdateFeedback = () => {
    updateFeedback.value = ''
  }

  return {
    systemInfo,
    checking,
    updateInfo,
    checkingUpdate,
    applyingUpdate,
    updateProgress,
    updateDialogOpen,
    updateFeedback,
    hasUpdate,
    startWatching,
    stopWatching,
    refreshSystem,
    checkUpdate,
    applyUpdate,
    dismissUpdate,
    openUpdateRelease,
    openRepository,
    clearUpdateFeedback,
  }
})
