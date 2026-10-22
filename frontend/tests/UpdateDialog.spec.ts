import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import UpdateDialog from '@/views/UpdateDialog.vue'

describe('UpdateDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('renders detected new version concisely and emits download or dismiss actions', async () => {
    const wrapper = mount(UpdateDialog, {
      attachTo: document.body,
      props: {
        open: true,
        updateInfo: {
          currentVersion: '0.1.0',
          latestVersion: '0.1.1',
          hasUpdate: true,
          releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.1.1',
        },
      },
    })
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('发现新版本')
    expect(document.body.textContent).toContain('检测到新版本 v0.1.1 可供下载')
    expect(document.body.textContent).toContain('v0.1.0')
    expect(
      document.body.querySelector('[data-testid="update-latest-version"]')?.textContent,
    ).toContain('v0.1.1')

    const downloadBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="update-download-btn"]',
    )
    const dismissBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="update-dismiss-btn"]',
    )
    expect(downloadBtn).not.toBeNull()
    expect(dismissBtn).not.toBeNull()

    downloadBtn?.click()
    dismissBtn?.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('openRelease')).toEqual([
      ['https://github.com/maybk/tunnelkit/releases/tag/v0.1.1'],
    ])
    expect(wrapper.emitted('dismissVersion')).toEqual([['0.1.1']])
    wrapper.unmount()
  })
})
