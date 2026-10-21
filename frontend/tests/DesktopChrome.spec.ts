import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

const wailsWindow = vi.hoisted(() => ({
  IsFocused: vi.fn(async () => true),
  IsMaximised: vi.fn(async () => false),
  IsFullscreen: vi.fn(async () => false),
  Minimise: vi.fn(async () => undefined),
  ToggleMaximise: vi.fn(async () => undefined),
  Close: vi.fn(async () => undefined),
}))

vi.mock('@wailsio/runtime', () => ({
  System: {
    IsWindows: () => systemMock.isWindows,
    IsMac: () => systemMock.isMac,
    IsLinux: () => systemMock.isLinux,
  },
  Window: wailsWindow,
  Events: {
    On: vi.fn(() => () => {}),
    Types: {
      Common: {
        WindowFocus: 'common:WindowFocus',
        WindowLostFocus: 'common:WindowLostFocus',
        WindowMaximise: 'common:WindowMaximise',
        WindowUnMaximise: 'common:WindowUnMaximise',
        WindowFullscreen: 'common:WindowFullscreen',
        WindowUnFullscreen: 'common:WindowUnFullscreen',
      },
    },
  },
}))

import DesktopChrome from '@/components/common/DesktopChrome.vue'
import { detectDesktopPlatform } from '@/api/window'
import { useDesktopWindow } from '@/stores/useDesktopWindow'
import { useNotice } from '@/stores/useNotice'

const systemMock = vi.hoisted(() => ({
  isWindows: false,
  isMac: true,
  isLinux: false,
}))

describe('DesktopChrome', () => {
  it('renders a shared application bar in Web Preview without desktop insets or caption buttons', () => {
    const wrapper = mount(DesktopChrome, {
      props: { platform: 'web', title: 'TunnelKit', isMaximised: false },
      slots: { default: '<span>全局上下文</span>' },
    })

    const header = wrapper.get('header')
    expect(header.attributes('data-platform')).toBe('web')
    expect(header.attributes('title')).toBe('TunnelKit')
    expect(wrapper.text()).toContain('全局上下文')
    expect(wrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)
    expect(wrapper.find('fieldset[aria-label="窗口控制"]').exists()).toBe(false)
  })

  it('reserves the macOS traffic-light inset in windowed mode and collapses it in fullscreen', async () => {
    const wrapper = mount(DesktopChrome, {
      props: { platform: 'macos', title: 'TunnelKit', isFullscreen: false },
      slots: { default: '<span>节点</span>' },
    })

    const dragRegion = wrapper.get('[data-testid="desktop-chrome-drag-region"]')
    expect((dragRegion.element as HTMLElement).style.getPropertyValue('--wails-draggable')).toBe(
      'drag',
    )
    expect(wrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(true)
    expect(wrapper.find('fieldset[aria-label="窗口控制"]').exists()).toBe(false)

    await wrapper.setProps({ isFullscreen: true })
    expect(wrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)
  })

  it('renders Windows native hit-test caption controls and delegates window actions', async () => {
    const wrapper = mount(DesktopChrome, {
      props: { platform: 'windows', title: 'TunnelKit', isMaximised: false },
    })

    const minimise = wrapper.get('button[aria-label="最小化窗口"]')
    const maximise = wrapper.get('button[aria-label="最大化窗口"]')
    const close = wrapper.get('button[aria-label="关闭窗口"]')
    const spacer = wrapper.get('[data-testid="desktop-chrome-caption-spacer"]')

    expect(
      (spacer.element as HTMLElement).style.getPropertyValue('--wails-non-client-region'),
    ).toBe('caption')
    expect(
      (minimise.element as HTMLElement).style.getPropertyValue('--wails-non-client-region'),
    ).toBe('minimize')
    expect(
      (maximise.element as HTMLElement).style.getPropertyValue('--wails-non-client-region'),
    ).toBe('maximize')
    expect((close.element as HTMLElement).style.getPropertyValue('--wails-non-client-region')).toBe(
      'close',
    )
    expect(minimise.classes()).toContain('size-12')

    await minimise.trigger('click')
    await maximise.trigger('click')
    await close.trigger('click')

    expect(wrapper.emitted('minimise')).toHaveLength(1)
    expect(wrapper.emitted('toggleMaximise')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)

    await wrapper.setProps({ isMaximised: true })
    expect(wrapper.find('button[aria-label="还原窗口"]').exists()).toBe(true)
  })

  it('distinguishes Linux WebKitGTK from macOS WKWebView before wails runtime config is ready', () => {
    systemMock.isMac = false
    systemMock.isWindows = false
    systemMock.isLinux = false

    const win = window as unknown as {
      webkit?: { messageHandlers?: { external?: { postMessage?: () => void } } }
    }
    const prevWebkit = win.webkit
    const prevUA = window.navigator.userAgent

    try {
      win.webkit = { messageHandlers: { external: { postMessage: () => {} } } }

      Object.defineProperty(window.navigator, 'userAgent', {
        value: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15',
        configurable: true,
      })
      expect(detectDesktopPlatform()).toBe('linux')

      Object.defineProperty(window.navigator, 'userAgent', {
        value: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15',
        configurable: true,
      })
      expect(detectDesktopPlatform()).toBe('macos')
    } finally {
      win.webkit = prevWebkit
      Object.defineProperty(window.navigator, 'userAgent', {
        value: prevUA,
        configurable: true,
      })
      systemMock.isMac = true
    }
  })

  it('forwards window action failures to the global notice store', async () => {
    const win = useDesktopWindow()
    const notice = useNotice()
    wailsWindow.Close.mockRejectedValueOnce(new Error('窗口关闭失败'))

    await win.close()
    expect(notice.message.value).toBe('窗口关闭失败')
  })
})
