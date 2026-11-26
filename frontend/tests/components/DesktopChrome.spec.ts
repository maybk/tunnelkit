import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vite-plus/test'
import DesktopChrome from '@/layouts/DesktopChrome.vue'

describe('DesktopChrome split titlebar contract', () => {
  it('adapts across macOS, Windows, and Linux with text-only brand and unified main-bar divider', async () => {
    const macWrapper = mount(DesktopChrome, {
      props: {
        platform: 'macos',
        brandTitle: 'TunnelKit',
        title: '总部网关',
        isFullscreen: false,
      },
    })
    expect(macWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(true)
    expect(macWrapper.get('[data-testid="desktop-chrome-brand"]').text()).toBe('TunnelKit')
    expect(macWrapper.find('[data-testid="desktop-chrome-sidebar-rail"] img').exists()).toBe(false)
    expect(macWrapper.get('[data-testid="page-title"]').text()).toBe('总部网关')
    expect(macWrapper.get('[data-testid="desktop-chrome-main-bar"]').classes()).toContain(
      'border-b',
    )
    expect(macWrapper.find('button[aria-label="最小化"]').exists()).toBe(false)

    expect(macWrapper.get('[data-testid="desktop-chrome-sidebar-rail"]').classes()).toContain(
      'w-64',
    )

    await macWrapper.setProps({
      isFullscreen: true,
      narrowRail: true,
      parentTitle: '设置',
      title: '通用',
    })
    expect(macWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)
    expect(macWrapper.get('[data-testid="desktop-chrome-sidebar-rail"]').classes()).toContain(
      'w-48',
    )
    expect(macWrapper.get('[data-testid="page-title"]').text()).toContain('设置')
    expect(macWrapper.get('[data-testid="page-title"]').text()).toContain('通用')
    macWrapper.unmount()

    const winWrapper = mount(DesktopChrome, {
      props: {
        platform: 'windows',
        brandTitle: 'TunnelKit',
        title: '新建连接',
        isMaximised: false,
      },
    })
    expect(winWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)
    expect(winWrapper.get('[data-testid="page-title"]').text()).toContain('新建连接')
    expect(winWrapper.find('button[aria-label="最小化"]').exists()).toBe(true)
    expect(winWrapper.find('button[aria-label="最大化"]').exists()).toBe(true)
    expect(winWrapper.find('button[aria-label="关闭窗口"]').exists()).toBe(true)

    await winWrapper.find('button[aria-label="最小化"]').trigger('click')
    await winWrapper.find('button[aria-label="最大化"]').trigger('click')
    await winWrapper.find('button[aria-label="关闭窗口"]').trigger('click')
    expect(winWrapper.emitted('minimise')).toHaveLength(1)
    expect(winWrapper.emitted('toggleMaximise')).toHaveLength(1)
    expect(winWrapper.emitted('close')).toHaveLength(1)

    await winWrapper.setProps({ isMaximised: true })
    expect(winWrapper.find('button[aria-label="向下还原"]').exists()).toBe(true)
    winWrapper.unmount()

    const linuxWrapper = mount(DesktopChrome, {
      props: {
        platform: 'linux',
        brandTitle: 'TunnelKit',
        title: '总部网关',
      },
    })
    expect(linuxWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)
    expect(linuxWrapper.find('button[aria-label="最小化"]').exists()).toBe(false)
    linuxWrapper.unmount()
  })
})
