import { ref } from 'vue'
import { defineStore } from 'pinia'
import { desktopWindowApi } from '@/api/window'
import { useNoticeStore } from '@/stores/useNoticeStore'
import type { DesktopPlatform } from '@/types/desktop'

const isDesktopRuntime = (platform: DesktopPlatform) => platform !== 'web'

export const useDesktopWindowStore = defineStore('desktop-window', () => {
  const noticeStore = useNoticeStore()

  const platform = ref<DesktopPlatform>(desktopWindowApi.platform)
  const isFocused = ref(true)
  const isMaximised = ref(false)
  const isFullscreen = ref(false)

  let stopListeners: (() => void) | null = null
  let stopRuntimeConfigListener: (() => void) | null = null

  const bindWindowSubscriptions = () => {
    platform.value = desktopWindowApi.platform
    if (!isDesktopRuntime(platform.value) || stopListeners) return

    void desktopWindowApi
      .isFocused()
      .then((v) => (isFocused.value = v))
      .catch(() => (isFocused.value = true))
    void desktopWindowApi
      .isMaximised()
      .then((v) => (isMaximised.value = v))
      .catch(() => (isMaximised.value = false))
    void desktopWindowApi
      .isFullscreen()
      .then((v) => (isFullscreen.value = v))
      .catch(() => (isFullscreen.value = false))

    const offFocus = desktopWindowApi.subscribeFocused((v) => (isFocused.value = v))
    const offMaximise = desktopWindowApi.subscribeMaximised((v) => (isMaximised.value = v))
    const offFullscreen = desktopWindowApi.subscribeFullscreen((v) => (isFullscreen.value = v))

    stopListeners = () => {
      offFocus()
      offMaximise()
      offFullscreen()
    }
  }

  const startWatching = () => {
    bindWindowSubscriptions()
    stopRuntimeConfigListener ??= desktopWindowApi.subscribeReady(bindWindowSubscriptions)
  }

  const stopWatching = () => {
    stopRuntimeConfigListener?.()
    stopRuntimeConfigListener = null
    stopListeners?.()
    stopListeners = null
  }

  const runWindowAction = async (action: () => Promise<void>) => {
    try {
      await action()
    } catch (err) {
      noticeStore.showError(err, '窗口操作失败')
    }
  }

  return {
    platform,
    isFocused,
    isMaximised,
    isFullscreen,
    startWatching,
    stopWatching,
    minimise: () => runWindowAction(() => desktopWindowApi.minimise()),
    toggleMaximise: () => runWindowAction(() => desktopWindowApi.toggleMaximise()),
    close: () => runWindowAction(() => desktopWindowApi.close()),
  }
})
