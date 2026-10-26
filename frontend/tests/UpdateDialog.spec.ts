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

  it('shows self-update button when canSelfUpdate is true and renders live progress when applying', async () => {
    const wrapper = mount(UpdateDialog, {
      attachTo: document.body,
      props: {
        open: true,
        updateInfo: {
          currentVersion: '0.2.0',
          latestVersion: '0.2.1',
          hasUpdate: true,
          releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.2.1',
          canSelfUpdate: true,
        },
        applying: false,
        progress: null,
      },
    })
    await wrapper.vm.$nextTick()

    const applyBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="update-apply-btn"]',
    )
    expect(applyBtn).not.toBeNull()
    expect(applyBtn?.textContent).toContain('立即更新并重启')

    applyBtn?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('applyUpdate')).toHaveLength(1)

    await wrapper.setProps({
      applying: true,
      progress: {
        stage: 'downloading',
        downloadedBytes: 4500000,
        totalBytes: 9000000,
        percent: 50,
        message: '正在下载更新安装包 (50%)…',
      },
    })
    await wrapper.vm.$nextTick()

    const progressBox = document.body.querySelector('[data-testid="update-progress"]')
    expect(progressBox?.textContent).toContain('正在下载更新安装包 (50%)…')
    expect(progressBox?.textContent).toContain('50%')
    expect(applyBtn?.disabled).toBe(true)

    const fallbackDownloadBtn = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="update-download-btn"]',
    )
    expect(fallbackDownloadBtn?.disabled).toBe(true)

    await wrapper.setProps({
      applying: false,
      progress: null,
    })
    await wrapper.vm.$nextTick()

    expect(fallbackDownloadBtn?.disabled).toBe(false)
    fallbackDownloadBtn?.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('openRelease')).toEqual([
      ['https://github.com/maybk/tunnelkit/releases/tag/v0.2.1'],
    ])
    wrapper.unmount()
  })
})
