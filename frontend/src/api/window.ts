import type { DesktopPlatform } from '@/types/desktop'
import { Events, System, Window } from '@wailsio/runtime'

interface NativeBridgeWindow extends globalThis.Window {
  webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } }
  chrome?: { webview?: { postMessage?: unknown } }
}

export function detectDesktopPlatform(): DesktopPlatform {
  try {
    if (System.IsWindows()) return 'windows'
    if (System.IsMac()) return 'macos'
    if (System.IsLinux()) return 'linux'
  } catch {
    // 忽略非 Wails 运行时异常
  }
  if (typeof window !== 'undefined') {
    const win = window as NativeBridgeWindow
    if (win.webkit?.messageHandlers?.external?.postMessage) {
      const ua = window.navigator?.userAgent ?? ''
      return /Linux|X11/i.test(ua) ? 'linux' : 'macos'
    }
    if (win.chrome?.webview?.postMessage) return 'windows'
  }
  return 'web'
}

export const desktopWindowApi = {
  get platform(): DesktopPlatform {
    return detectDesktopPlatform()
  },

  subscribeReady(listener: () => void): () => void {
    if (typeof window === 'undefined') return () => {}
    window.addEventListener('wails:runtime-config-ready', listener, { once: true })
    return () => window.removeEventListener('wails:runtime-config-ready', listener)
  },

  isFocused(): Promise<boolean> {
    return Window.IsFocused()
  },

  isMaximised(): Promise<boolean> {
    return Window.IsMaximised()
  },

  isFullscreen(): Promise<boolean> {
    return Window.IsFullscreen()
  },

  subscribeFocused(listener: (focused: boolean) => void): () => void {
    const offFocus = Events.On(Events.Types.Common.WindowFocus, () => listener(true))
    const offLostFocus = Events.On(Events.Types.Common.WindowLostFocus, () => listener(false))
    return () => {
      offFocus()
      offLostFocus()
    }
  },

  subscribeMaximised(listener: (maximised: boolean) => void): () => void {
    const offMaximise = Events.On(Events.Types.Common.WindowMaximise, () => listener(true))
    const offUnMaximise = Events.On(Events.Types.Common.WindowUnMaximise, () => listener(false))
    return () => {
      offMaximise()
      offUnMaximise()
    }
  },

  subscribeFullscreen(listener: (fullscreen: boolean) => void): () => void {
    const offFullscreen = Events.On(Events.Types.Common.WindowFullscreen, () => listener(true))
    const offUnFullscreen = Events.On(Events.Types.Common.WindowUnFullscreen, () => listener(false))
    return () => {
      offFullscreen()
      offUnFullscreen()
    }
  },

  minimise(): Promise<void> {
    return Window.Minimise()
  },

  toggleMaximise(): Promise<void> {
    return Window.ToggleMaximise()
  },

  close(): Promise<void> {
    return Window.Close()
  },
}
