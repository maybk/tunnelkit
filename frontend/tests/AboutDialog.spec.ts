import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('@/api/system', () => ({
  GITHUB_REPO_LABEL: 'github.com/maybk/tunnelkit',
  GITHUB_REPO_URL: 'https://github.com/maybk/tunnelkit',
}))

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
    expect(document.body.textContent).toContain('github.com/maybk/tunnelkit')
    expect(document.body.textContent).not.toContain('服务组件路径')
    expect(document.body.textContent).not.toContain('重新检测')
    expect(document.body.textContent).toContain('检查更新')

    const repoBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="about-open-repo"]',
    )
    expect(repoBtn).not.toBeNull()
    repoBtn?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('openRepo')).toHaveLength(1)
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

  it('emits checkUpdate or showUpdate and displays up-to-date feedback', async () => {
    const wrapper = mount(AboutDialog, {
      attachTo: document.body,
      props: {
        open: true,
        checking: false,
        systemInfo: {
          platform: 'darwin',
          appVersion: '0.1.0',
          helperReady: true,
        },
        updateFeedback: '当前已是最新版本',
      },
    })
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('当前已是最新版本')
    const checkBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="about-check-update"]',
    )
    expect(checkBtn).not.toBeNull()
    checkBtn?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('checkUpdate')).toHaveLength(1)

    await wrapper.setProps({
      updateInfo: {
        currentVersion: '0.1.0',
        latestVersion: '0.1.1',
        hasUpdate: true,
        releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.1.1',
      },
    })
    const showBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="about-show-update"]',
    )
    expect(showBtn?.textContent).toContain('新版本 v0.1.1')
    showBtn?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('showUpdate')).toHaveLength(1)
    wrapper.unmount()
  })
})
