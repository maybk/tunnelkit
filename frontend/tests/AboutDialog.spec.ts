import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AboutDialog from '@/views/AboutDialog.vue'

describe('AboutDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('displays concise status when helper is ready and hides recheck button', async () => {
    const wrapper = mount(AboutDialog, {
      attachTo: document.body,
      props: {
        open: true,
        checking: false,
        systemInfo: {
          platform: 'darwin',
          appVersion: '0.1.0',
          helperReady: true,
          helperPath: '/Users/test/very/long/path/tunnelkit.app/Contents/MacOS/tunnelkit-helper',
        },
      },
    })
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('v0.1.0')
    expect(document.body.textContent).toContain('已就绪')
    expect(document.body.textContent).not.toContain('服务组件路径')
    expect(document.body.textContent).not.toContain('重新检测')
    wrapper.unmount()
  })

  it('shows recheck button only when helper is not ready', async () => {
    const wrapper = mount(AboutDialog, {
      attachTo: document.body,
      props: {
        open: true,
        checking: false,
        systemInfo: {
          platform: 'darwin',
          appVersion: '0.1.0',
          helperReady: false,
          helperPath: '',
        },
      },
    })
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('未就绪')
    const recheckBtn = Array.from(document.body.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('重新检测'),
    )
    expect(recheckBtn).toBeDefined()
    recheckBtn?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('recheck')).toHaveLength(1)
    wrapper.unmount()
  })
})
