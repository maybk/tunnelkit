import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import UpdateDialog from '@/components/common/UpdateDialog.vue'
import { Dialog } from '@/components/ui/dialog'
import WorkspaceView from '@/views/WorkspaceView.vue'
import { createTestProfile } from '../fixtures/profiles'

describe('UpdateDialog and WorkspaceView components', () => {
  it('renders UpdateDialog when updateInfo is provided', async () => {
    const wrapper = mount(UpdateDialog, {
      props: {
        open: true,
        updateInfo: {
          currentVersion: '0.3.2',
          latestVersion: '0.4.0',
          hasUpdate: true,
          releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.4.0',
          canSelfUpdate: true,
          assetName: 'tunnelkit-darwin-arm64.tar.gz',
          assetUrl: 'https://github.com/maybk/tunnelkit/releases/download/v0.4.0/tunnelkit.tar.gz',
          assetSize: 4096,
          assetSha256: 'abc123',
        },
        progress: null,
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
    expect(wrapper.text()).toContain('发现新版本')
    expect(wrapper.find('[data-testid="update-latest-version"]').text()).toBe('v0.4.0')
    expect(wrapper.text()).toContain('立即更新')
    expect(wrapper.find('[data-testid="update-release-link"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="update-download-btn"]').exists()).toBe(false)

    await wrapper.find('[data-testid="update-release-link"]').trigger('click')
    expect(wrapper.emitted('openRelease')).toEqual([
      ['https://github.com/maybk/tunnelkit/releases/tag/v0.4.0'],
    ])

    await wrapper.find('[data-testid="update-apply-btn"]').trigger('click')
    expect(wrapper.emitted('downloadUpdate')).toHaveLength(1)

    await wrapper.find('[data-testid="update-dismiss-btn"]').trigger('click')
    expect(wrapper.emitted('dismissVersion')).toEqual([['0.4.0']])

    await wrapper.setProps({ opening: true })
    expect(
      wrapper.find<HTMLButtonElement>('[data-testid="update-release-link"]').element.disabled,
    ).toBe(true)
    expect(
      wrapper.find<HTMLButtonElement>('[data-testid="update-apply-btn"]').element.disabled,
    ).toBe(true)
    expect(
      wrapper.find<HTMLButtonElement>('[data-testid="update-dismiss-btn"]').element.disabled,
    ).toBe(true)

    wrapper.findComponent(Dialog).vm.$emit('update:open', false)
    expect(wrapper.emitted('update:open')).toEqual([[false]])
    wrapper.unmount()
  })

  it('switches primary action to manual download when self-update is unavailable and focuses dialog container on open', async () => {
    const wrapper = mount(UpdateDialog, {
      props: {
        open: true,
        updateInfo: {
          currentVersion: '1.0.0',
          latestVersion: '1.0.1',
          hasUpdate: true,
          releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.0.1',
          canSelfUpdate: false,
          assetName: '',
          assetUrl: '',
          assetSize: 0,
          assetSha256: '',
        },
        progress: null,
      },
      attachTo: document.body,
    })
    await nextTick()
    await nextTick()

    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog).not.toBeNull()
    expect(document.activeElement).toBe(dialog)
    expect(dialog.textContent).toContain('请前往发布页下载安装包完成更新')
    expect(dialog.querySelector('[data-testid="update-release-link"]')).toBeNull()
    expect(dialog.querySelector('[data-testid="update-apply-btn"]')).toBeNull()

    const downloadBtn = dialog.querySelector<HTMLButtonElement>(
      '[data-testid="update-download-btn"]',
    )!
    expect(downloadBtn).not.toBeNull()
    expect(downloadBtn.disabled).toBe(false)
    downloadBtn.click()
    await nextTick()
    expect(wrapper.emitted('openRelease')).toEqual([
      ['https://github.com/maybk/tunnelkit/releases/tag/v1.0.1'],
    ])
    wrapper.unmount()
  })

  it('refuses all dialog close requests while locked in downloading/verifying/installing phases', () => {
    const wrapper = mount(UpdateDialog, {
      props: { open: true, phase: 'downloading', updateInfo: null, progress: null },
      global: { stubs: { Dialog: { props: ['open'], template: '<div />' } } },
    })
    wrapper.findComponent(Dialog).vm.$emit('update:open', false)
    expect(wrapper.emitted('update:open')).toBeUndefined()
    wrapper.unmount()
  })

  it('renders Sparkle-style downloading, verifying, ready, and installing states with proper controls', async () => {
    const updateInfo = {
      currentVersion: '1.1.2',
      latestVersion: '1.1.3',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.1.3',
      canSelfUpdate: true,
      assetName: 'tunnelkit-1.1.3-darwin-arm64.dmg',
      assetUrl:
        'https://github.com/maybk/tunnelkit/releases/download/v1.1.3/tunnelkit-1.1.3-darwin-arm64.dmg',
      assetSize: 16 * 1024 * 1024,
      assetSha256: 'abc123',
    }
    const wrapper = mount(UpdateDialog, {
      props: {
        open: true,
        phase: 'downloading',
        updateInfo,
        vpnActive: true,
        progress: {
          stage: 'downloading',
          downloadedBytes: 8 * 1024 * 1024,
          totalBytes: 16 * 1024 * 1024,
          percent: 50,
          message: '正在下载更新…',
        },
      },
      attachTo: document.body,
    })
    await nextTick()
    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog).not.toBeNull()
    expect(dialog.textContent).toContain('正在下载更新…')
    expect(dialog.querySelector('[data-testid="update-progress-meta"]')?.textContent).toContain(
      '8.0 MB / 16.0 MB',
    )
    expect(dialog.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBe('50')
    expect(dialog.querySelector('[aria-label="关闭"]')).toBeNull()

    const cancelBtn = dialog.querySelector<HTMLButtonElement>('[data-testid="update-cancel-btn"]')!
    expect(cancelBtn).not.toBeNull()
    expect(cancelBtn.disabled).toBe(false)
    cancelBtn.click()
    await nextTick()
    expect(wrapper.emitted('cancelUpdate')).toHaveLength(1)

    dialog.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    )
    await nextTick()
    expect(wrapper.emitted('update:open')).toBeUndefined()

    // Verifying disables cancel button while keeping dialog locked.
    await wrapper.setProps({
      phase: 'verifying',
      progress: {
        stage: 'verifying',
        downloadedBytes: 16 * 1024 * 1024,
        totalBytes: 16 * 1024 * 1024,
        percent: 100,
        message: '正在校验更新…',
      },
    })
    expect(dialog.textContent).toContain('正在校验更新…')
    expect(
      dialog.querySelector<HTMLButtonElement>('[data-testid="update-cancel-btn"]')?.disabled,
    ).toBe(true)

    // Ready state unlocks close button, warns about active VPN, and exposes defer + install actions.
    await wrapper.setProps({
      phase: 'ready',
      progress: {
        stage: 'ready',
        downloadedBytes: 16 * 1024 * 1024,
        totalBytes: 16 * 1024 * 1024,
        percent: 100,
        message: '可以开始安装了',
      },
    })
    expect(dialog.textContent).toContain('可以开始安装了')
    expect(dialog.querySelector('[data-testid="update-latest-version"]')).toBeNull()
    expect(dialog.textContent).toContain('将先断开当前 VPN')
    expect(dialog.querySelector('[aria-label="关闭"]')).not.toBeNull()

    dialog.querySelector<HTMLButtonElement>('[data-testid="update-defer-btn"]')!.click()
    dialog.querySelector<HTMLButtonElement>('[data-testid="update-install-btn"]')!.click()
    await nextTick()
    expect(wrapper.emitted('deferInstall')).toHaveLength(1)
    expect(wrapper.emitted('installUpdate')).toHaveLength(1)

    // Installing locks dialog again without close or cancel button.
    await wrapper.setProps({
      phase: 'installing',
      progress: {
        stage: 'installing',
        downloadedBytes: 16 * 1024 * 1024,
        totalBytes: 16 * 1024 * 1024,
        percent: 100,
        message: '正在安装更新…',
      },
    })
    expect(dialog.textContent).toContain('正在安装更新…')
    expect(dialog.querySelector('[aria-label="关闭"]')).toBeNull()
    expect(dialog.querySelector('[data-testid="update-cancel-btn"]')).toBeNull()
    wrapper.unmount()
  })

  it('renders EmptyState, helper missing warning, and workbench in WorkspaceView', async () => {
    vi.spyOn(profileApi, 'list').mockResolvedValue([createTestProfile()])

    const { useSystemStore } = await import('@/stores/useSystemStore')
    const systemStore = useSystemStore()
    systemStore.systemInfo = {
      appVersion: '0.3.2',
      platform: 'darwin',
      helperReady: false,
      helperPath: '',
      dataDir: '',
    }

    const wrapperEmpty = mount(WorkspaceView)
    expect(wrapperEmpty.text()).toContain('添加第一个 VPN 连接')
    expect(wrapperEmpty.find('[data-testid="helper-missing-banner"]').exists()).toBe(true)

    const { useProfilesStore } = await import('@/stores/useProfilesStore')
    const profilesStore = useProfilesStore()
    await profilesStore.loadProfiles()
    profilesStore.setActiveTab('config')
    await nextTick()

    const wrapperWithProfile = mount(WorkspaceView)
    expect(wrapperWithProfile.get('[data-testid="hero-status-title"]').text()).toBe('未连接')
    expect(wrapperWithProfile.text()).toContain('vpn.example.com:443')

    const nameInput = wrapperWithProfile.find('[data-testid="profile-name-input"]')
    await nameInput.setValue('未保存草稿')
    profilesStore.setActiveTab('status')
    await nextTick()
    profilesStore.setActiveTab('config')
    await nextTick()
    expect(
      wrapperWithProfile.find<HTMLInputElement>('[data-testid="profile-name-input"]').element.value,
    ).toBe('未保存草稿')
  })
})
