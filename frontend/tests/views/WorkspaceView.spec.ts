import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import UpdateDialog from '@/components/common/UpdateDialog.vue'
import { Dialog } from '@/components/ui/dialog'
import WorkspaceView from '@/views/WorkspaceView.vue'
import { createTestProfile } from '../fixtures/profiles'

describe('UpdateDialog and WorkspaceView components', () => {
  it('renders UpdateDialog when updateInfo is provided', () => {
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
    expect(wrapper.text()).toContain('v0.4.0')
    expect(wrapper.text()).toContain('立即更新并重启')
    wrapper.findComponent(Dialog).vm.$emit('update:open', false)
    expect(wrapper.emitted('update:open')).toEqual([[false]])
    wrapper.unmount()
  })

  it('refuses all dialog close requests while applying an update', () => {
    const wrapper = mount(UpdateDialog, {
      props: { open: true, applying: true, updateInfo: null, progress: null },
      global: { stubs: { Dialog: { props: ['open'], template: '<div />' } } },
    })
    wrapper.findComponent(Dialog).vm.$emit('update:open', false)
    expect(wrapper.emitted('update:open')).toBeUndefined()
    wrapper.unmount()
  })

  it('keeps an in-progress update open without offering an ineffective close button', async () => {
    const wrapper = mount(UpdateDialog, {
      props: { open: true, applying: true, updateInfo: null, progress: null },
      attachTo: document.body,
    })
    await nextTick()
    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog).not.toBeNull()
    expect(dialog.textContent).toContain('正在更新')
    expect(dialog.textContent).not.toContain('可供下载')
    expect(dialog.querySelector('[data-testid="update-dismiss-btn"]')).toBeNull()
    expect(dialog.querySelector('[data-testid="update-download-btn"]')).toBeNull()
    expect(dialog.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBe('0')
    expect(dialog.querySelector('[aria-label="关闭"]')).toBeNull()
    dialog.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    )
    await nextTick()
    expect(wrapper.emitted('update:open')).toBeUndefined()
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    await wrapper.setProps({ applying: false })
    expect(dialog.textContent).toContain('发现新版本')
    expect(dialog.querySelector('[data-testid="update-dismiss-btn"]')).not.toBeNull()
    expect(dialog.querySelector('[data-testid="update-download-btn"]')).not.toBeNull()
    expect(dialog.querySelector('[aria-label="关闭"]')).not.toBeNull()
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
    expect(wrapperWithProfile.text()).toContain('总部网关')
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
