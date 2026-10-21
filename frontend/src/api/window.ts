import type { CSSProperties } from 'vue'
import { Events, System, Window } from '@wailsio/runtime'

export type DesktopPlatform = 'macos' | 'windows' | 'linux' | 'web'
export type WailsWindowControlRegion = 'minimize' | 'maximize' | 'close'

export interface DesktopWindowPort {
  readonly platform: DesktopPlatform
  isFocused(): Promise<boolean>
  isMaximised(): Promise<boolean>
  isFullscreen(): Promise<boolean>
  subscribeFocused(listener: (isFocused: boolean) => void): () => void
  subscribeMaximised(listener: (isMaximised: boolean) => void): () => void
  subscribeFullscreen(listener: (isFullscreen: boolean) => void): () => void
  minimise(): Promise<void>
  toggleMaximise(): Promise<void>
  close(): Promise<void>
}

/** 传统 Wails 拖拽契约，允许子元素通过 no-drag 退出拖拽命中。 */
export const windowDragRegionStyle: CSSProperties = {
  '--wails-draggable': 'drag',
}

/** 交互控件显式退出窗口拖拽区。 */
export const windowInteractiveRegionStyle: CSSProperties = {
  '--wails-draggable': 'no-drag',
  '--wails-non-client-region': 'none',
}

/**
 * Windows Composition Hosting 将该元素上报为原生 HTCAPTION 矩形。
 * 仅用于纯空白弹性拖拽段，避免自定义属性继承给子交互控件。
 */
export const windowCaptionRegionStyle: CSSProperties = {
  '--wails-draggable': 'drag',
  '--wails-non-client-region': 'caption',
}

/** Windows 原生 caption 控件保留 WebView2 非客户区命中测试语义（含 Snap Layout）。 */
export const windowControlRegionStyle = (region: WailsWindowControlRegion): CSSProperties => ({
  '--wails-draggable': 'no-drag',
  '--wails-non-client-region': region,
})

interface NativeBridgeWindow extends globalThis.Window {
  webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } }
  chrome?: { webview?: { postMessage?: unknown } }
}

export const detectDesktopPlatform = (): DesktopPlatform => {
  if (System.IsWindows()) return 'windows'
  if (System.IsMac()) return 'macos'
  if (System.IsLinux()) return 'linux'
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

export const desktopWindowApi: DesktopWindowPort = {
  get platform() {
    return detectDesktopPlatform()
  },
  isFocused: () => Window.IsFocused(),
  isMaximised: () => Window.IsMaximised(),
  isFullscreen: () => Window.IsFullscreen(),
  subscribeFocused: (listener) => {
    const offFocus = Events.On(Events.Types.Common.WindowFocus, () => listener(true))
    const offLostFocus = Events.On(Events.Types.Common.WindowLostFocus, () => listener(false))
    return () => {
      offFocus()
      offLostFocus()
    }
  },
  subscribeMaximised: (listener) => {
    const offMaximise = Events.On(Events.Types.Common.WindowMaximise, () => listener(true))
    const offUnMaximise = Events.On(Events.Types.Common.WindowUnMaximise, () => listener(false))
    return () => {
      offMaximise()
      offUnMaximise()
    }
  },
  subscribeFullscreen: (listener) => {
    const offFullscreen = Events.On(Events.Types.Common.WindowFullscreen, () => listener(true))
    const offUnFullscreen = Events.On(Events.Types.Common.WindowUnFullscreen, () => listener(false))
    return () => {
      offFullscreen()
      offUnFullscreen()
    }
  },
  minimise: () => Window.Minimise(),
  toggleMaximise: () => Window.ToggleMaximise(),
  close: () => Window.Close(),
}
