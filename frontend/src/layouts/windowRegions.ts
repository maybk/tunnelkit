import type { CSSProperties } from 'vue'

export type WailsWindowControlRegion = 'minimize' | 'maximize' | 'close'

/** 传统 Wails 拖拽契约，允许子元素通过 no-drag 退出拖拽命中。 */
export const windowDragRegionStyle: CSSProperties = {
  '--wails-draggable': 'drag',
}

/** 交互控件显式退出窗口拖拽区与非客户区。 */
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
