import { ref } from 'vue'
import { createGlobalState } from '@vueuse/core'
import { desktopWindowApi, detectDesktopPlatform, type DesktopPlatform } from '@/api/window'
import { useNotice } from '@/stores/useNotice'

const RUNTIME_CONFIG_READY_EVENT = 'wails:runtime-config-ready'

const isDesktopRuntime = (platform: DesktopPlatform) => platform !== 'web'

export const useDesktopWindow = createGlobalState(() => {
  const { showError } = useNotice()

  const platform = ref<DesktopPlatform>(desktopWindowApi.platform)
  const isFocused = ref(true)
  const isMaximised = ref(false)
  const isFullscreen = ref(false)

  let stopListeners: (() => void) | null = null
  let stopRuntimeConfigListener: (() => void) | null = null

  const bindWindowSubscriptions = () => {
    platform.value = detectDesktopPlatform()
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
    if (typeof window !== 'undefined' && !stopRuntimeConfigListener) {
      const onConfigReady = () => bindWindowSubscriptions()
      window.addEventListener(RUNTIME_CONFIG_READY_EVENT, onConfigReady, { once: true })
      stopRuntimeConfigListener = () =>
        window.removeEventListener(RUNTIME_CONFIG_READY_EVENT, onConfigReady)
    }
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
      showError(err)
    }
  }

  return {
    platform,
    isFocused,
    isMaximised,
    isFullscreen,
    startWatching,
    stopWatching,
    minimise: () => runWindowAction(desktopWindowApi.minimise),
    toggleMaximise: () => runWindowAction(desktopWindowApi.toggleMaximise),
    close: () => runWindowAction(desktopWindowApi.close),
  }
})
