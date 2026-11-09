import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vite-plus/test'
import AboutDialog from '@/components/common/AboutDialog.vue'
import DesktopChrome from '@/layouts/DesktopChrome.vue'

describe('DesktopChrome and AboutDialog', () => {
  it('adapts DesktopChrome across macOS, Windows, and Linux platforms', async () => {
    const macWrapper = mount(DesktopChrome, {
      props: {
        platform: 'macos',
        title: 'TunnelKit',
        statusLabel: '已连接',
        statusTone: 'success',
        isFullscreen: false,
      },
    })
    expect(macWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(true)
    expect(macWrapper.find('button[aria-label="最小化"]').exists()).toBe(false)

    await macWrapper.setProps({ isFullscreen: true })
    expect(macWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)

    const winWrapper = mount(DesktopChrome, {
      props: {
        platform: 'windows',
        title: 'TunnelKit',
        statusLabel: '未连接',
        statusTone: 'neutral',
        isMaximised: false,
      },
    })
    expect(winWrapper.find('[data-testid="desktop-chrome-leading-inset"]').exists()).toBe(false)
    expect(winWrapper.find('button[aria-label="最小化"]').exists()).toBe(true)
    expect(winWrapper.find('button[aria-label="最大化"]').exists()).toBe(true)
    expect(winWrapper.find('button[aria-label="关闭窗口"]').exists()).toBe(true)

    await winWrapper.find('button[aria-label="最小化"]').trigger('click')
    await winWrapper.find('[data-testid="chrome-about-btn"]').trigger('click')
    expect(winWrapper.emitted('minimise')).toHaveLength(1)
    expect(winWrapper.emitted('openAbout')).toHaveLength(1)
  })

  it('renders AboutDialog with system info and repository link', async () => {
    const wrapper = mount(AboutDialog, {
      props: {
        open: true,
        appVersion: '0.3.2',
        systemInfo: {
          appVersion: '0.3.2',
          platform: 'darwin',
          helperReady: true,
          helperPath: '/Library/PrivilegedHelperTools/tunnelkit-helper',
        },
        updateInfo: null,
      },
      global: {
        stubs: {
          Dialog: { template: '<div><slot /></div>' },
          AppDialogContent: { template: '<div><slot /></div>' },
          DialogHeader: { template: '<div><slot /></div>' },
          AppDialogTitle: { template: '<div><slot /></div>' },
          AppDialogDescription: { template: '<div><slot /></div>' },
          DialogFooter: { template: '<div><slot /></div>' },
        },
      },
    })
    expect(wrapper.text()).toContain('fortivpn 原生引擎')
    expect(wrapper.text()).toContain('已就绪')
    expect(wrapper.text()).toContain('github.com/maybk/tunnelkit')

    await wrapper.find('[data-testid="about-open-repo"]').trigger('click')
    expect(wrapper.emitted('openRepo')).toHaveLength(1)
  })
})
