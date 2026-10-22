import { computed, ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import {
  checkSystemUpdate,
  getSystemInfo,
  openGitHubRepo,
  openReleaseUrl,
  type SystemInfo,
  type UpdateInfo,
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
  const updateDialogOpen = ref(false)
  const updateFeedback = ref('')

  let autoCheckStarted = false

  const hasUpdate = computed(() => Boolean(updateInfo.value?.hasUpdate))

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
      if (autoCheckStarted) return updateInfo.value
      autoCheckStarted = true
    } else {
      if (checkingUpdate.value) return updateInfo.value
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

  const dismissUpdate = (version?: string) => {
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
    updateDialogOpen.value = false
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
    updateDialogOpen,
    updateFeedback,
    hasUpdate,
    refreshSystem,
    checkUpdate,
    dismissUpdate,
    openUpdateRelease,
    openRepository,
    clearUpdateFeedback,
  }
})
