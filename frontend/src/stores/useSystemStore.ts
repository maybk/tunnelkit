import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { systemApi } from '@/api/system'
import { GITHUB_RELEASES_URL, GITHUB_REPO_URL } from '@/constants/releases'
import { DISMISSED_UPDATE_VERSION_KEY } from '@/constants/storage'
import { useNoticeStore } from '@/stores/useNoticeStore'
import type { SystemInfo, UpdateInfo, UpdateProgress } from '@/types/system'

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
  const aboutDialogOpen = ref(false)
  const checkingSystem = ref(false)
  const isCheckingUpdate = ref(false)
  const isApplyingUpdate = ref(false)
  const updateFeedback = ref('')
  const hasCheckedUpdateOnce = ref(false)
  const dismissedVersion = ref<string>(readDismissedVersion())

  let stopProgressSub: (() => void) | null = null

  const appVersion = computed(
    () => systemInfo.value?.appVersion || updateInfo.value?.currentVersion || '',
  )

  const hasUpdate = computed(() => Boolean(updateInfo.value?.hasUpdate))

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
      updateProgress.value = progress
    })
  }

  const stopWatching = () => {
    stopProgressSub?.()
    stopProgressSub = null
  }

  const checkUpdate = async (options: { silent?: boolean } = {}): Promise<UpdateInfo | null> => {
    if (isCheckingUpdate.value) return updateInfo.value
    if (options.silent && hasCheckedUpdateOnce.value) return updateInfo.value

    hasCheckedUpdateOnce.value = true
    isCheckingUpdate.value = true
    if (!options.silent) {
      updateFeedback.value = ''
    }

    try {
      const info = await systemApi.checkUpdate()
      updateInfo.value = info
      if (info.hasUpdate) {
        if (!options.silent) {
          aboutDialogOpen.value = false
          updateDialogOpen.value = true
        } else {
          const isDismissed =
            Boolean(dismissedVersion.value) && dismissedVersion.value === info.latestVersion
          if (!isDismissed) {
            updateDialogOpen.value = true
          }
        }
      } else if (!options.silent) {
        updateFeedback.value = '当前已是最新版本'
        noticeStore.showSuccess(
          '当前已是最新版本',
          info.currentVersion ? `v${info.currentVersion}` : undefined,
        )
      }
      return info
    } catch (err) {
      if (!options.silent) {
        noticeStore.showError(err, '检查更新失败')
      }
      return null
    } finally {
      isCheckingUpdate.value = false
    }
  }

  const applyUpdate = async () => {
    if (isApplyingUpdate.value) return
    isApplyingUpdate.value = true
    updateProgress.value = {
      stage: 'preparing',
      downloadedBytes: 0,
      totalBytes: 0,
      percent: 0,
      message: '正在准备下载更新包…',
    }
    try {
      await systemApi.applyUpdate()
    } catch (err) {
      isApplyingUpdate.value = false
      updateProgress.value = null
      noticeStore.showError(err, '自动更新失败')
    }
  }

  const dismissUpdate = (version?: string) => {
    const target = version || updateInfo.value?.latestVersion || ''
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

  const openUpdateFromAbout = () => {
    aboutDialogOpen.value = false
    updateDialogOpen.value = true
  }

  const openReleasePage = async (url?: string) => {
    const targetUrl = url || updateInfo.value?.releaseUrl || GITHUB_RELEASES_URL
    try {
      await systemApi.openExternalUrl(targetUrl)
    } catch (err) {
      noticeStore.showError(err, '打开版本发布页面失败')
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
    aboutDialogOpen,
    checkingSystem,
    isCheckingUpdate,
    isApplyingUpdate,
    updateFeedback,
    appVersion,
    hasUpdate,
    loadSystemInfo,
    startWatching,
    stopWatching,
    checkUpdate,
    applyUpdate,
    dismissUpdate,
    openUpdateFromAbout,
    openReleasePage,
    openRepository,
  }
})
