import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import { systemApi } from '@/api/system'
import WorkbenchLayout from '@/layouts/WorkbenchLayout.vue'
import { router } from '@/router'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSettingsStore } from '@/stores/useSettingsStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { createTestProfile } from '../fixtures/profiles'

describe('Settings views and WorkbenchLayout integration', () => {
  let wrapper: VueWrapper | null = null

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    vi.useRealTimers()
  })

  it('switches the entire sidebar into SettingsSidebar, remembers last settings tab, and supports Esc and Mod+, shortcuts', async () => {
    const profilesStore = useProfilesStore()
    profilesStore.profiles = [createTestProfile()]
    profilesStore.selectProfile('profile-1')

    const systemStore = useSystemStore()
    systemStore.systemInfo = {
      appVersion: '1.1.5',
      platform: 'darwin',
      helperReady: true,
      helperPath: '/Library/PrivilegedHelperTools/tunnelkit-helper',
      dataDir: '/Users/alice/.tunnelkit',
    }

    vi.spyOn(profileApi, 'saveSettings').mockImplementation(async (patch) => ({
      theme: patch.theme ?? 'system',
      autoCheckUpdate: patch.autoCheckUpdate ?? true,
    }))

    wrapper = mount(WorkbenchLayout, { attachTo: document.body })
    await flushPromises()

    // 主工作区选中连接时，右侧顶栏展示当前连接名称。
    expect(wrapper.find('[data-testid="app-sidebar"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-sidebar"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="page-title"]').text()).toBe('总部网关')

    // 点击底部「设置」整栏切换为设置侧栏，默认进入「通用」。
    await wrapper.get('[data-testid="sidebar-settings-btn"]').trigger('click')
    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe('/settings/general')
      expect(wrapper!.find('[data-testid="settings-sidebar"]').exists()).toBe(true)
      expect(wrapper!.find('[data-testid="auto-check-update-switch"]').exists()).toBe(true)
    })
    expect(wrapper.find('[data-testid="app-sidebar"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="settings-sidebar"]').classes()).toContain('w-48')
    expect(wrapper.get('[data-testid="desktop-chrome-sidebar-rail"]').classes()).toContain('w-48')
    expect(wrapper.get('[data-testid="page-title"]').text()).toContain('设置')
    expect(wrapper.get('[data-testid="page-title"]').text()).toContain('通用')

    // 切换「自动检查更新」开关。
    await wrapper.get('[data-testid="auto-check-update-switch"]').trigger('click')
    await flushPromises()
    expect(useSettingsStore().settings.autoCheckUpdate).toBe(false)

    // 切换到「外观」并修改主题模式。
    await wrapper.get('[data-testid="settings-nav-appearance"]').trigger('click')
    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe('/settings/appearance')
      expect(wrapper!.find('[data-testid="settings-theme-dark"]').exists()).toBe(true)
    })
    expect(wrapper.get('[data-testid="page-title"]').text()).toContain('外观')
    await wrapper.get('[data-testid="settings-theme-dark"]').trigger('click')
    await flushPromises()
    expect(useSettingsStore().settings.theme).toBe('dark')

    // 按 Esc 返回主工作区，再用 Mod+, 重进设置，应记住停留在「外观」。
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe('/workspace')
      expect(wrapper!.find('[data-testid="app-sidebar"]').exists()).toBe(true)
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: ',', metaKey: true, bubbles: true }))
    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe('/settings/appearance')
      expect(wrapper!.find('[data-testid="settings-sidebar"]').exists()).toBe(true)
    })

    // 点「← 返回」回到主工作区。
    await wrapper.get('[data-testid="settings-back-btn"]').trigger('click')
    await vi.waitFor(() => {
      expect(router.currentRoute.value.path).toBe('/workspace')
    })
  })

  it('renders AboutView info, copies dataDir, runs inline update check, and opens UpdateDialog when available', async () => {
    const systemStore = useSystemStore()
    systemStore.systemInfo = {
      appVersion: '1.1.5',
      platform: 'darwin',
      helperReady: true,
      helperPath: '/Library/PrivilegedHelperTools/tunnelkit-helper',
      dataDir: '/Users/alice/.tunnelkit',
    }

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    const openRepoSpy = vi.spyOn(systemApi, 'openExternalUrl').mockResolvedValue()
    const checkSpy = vi.spyOn(systemApi, 'checkUpdate').mockResolvedValue({
      currentVersion: '1.1.5',
      latestVersion: '1.1.5',
      hasUpdate: false,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.1.5',
      canSelfUpdate: false,
      assetName: '',
      assetUrl: '',
      assetSize: 0,
      assetSha256: '',
    })

    await router.push('/settings/about')
    wrapper = mount(WorkbenchLayout, { attachTo: document.body })
    await vi.waitFor(() => {
      expect(wrapper!.find('[data-testid="about-version-badge"]').exists()).toBe(true)
    })

    expect(wrapper.get('[data-testid="about-version-badge"]').text()).toBe('v1.1.5')
    expect(wrapper.text()).toContain('已就绪')
    expect(wrapper.get('[data-testid="about-data-dir"]').text()).toBe('~/.tunnelkit/')
    expect(wrapper.text()).toContain('github.com/maybk/tunnelkit')

    await wrapper.get('[data-testid="about-copy-data-dir"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('/Users/alice/.tunnelkit')

    const checkBtn = wrapper.get('[data-testid="about-check-update"]')
    await checkBtn.trigger('click')
    await flushPromises()
    expect(checkSpy).toHaveBeenCalledOnce()
    expect(checkBtn.attributes('data-state')).toBe('up-to-date')
    expect(checkBtn.text()).toContain('已是最新版本')

    await wrapper.get('[data-testid="about-open-repo"]').trigger('click')
    await flushPromises()
    expect(openRepoSpy).toHaveBeenCalledWith('https://github.com/maybk/tunnelkit')

    // 存在可用更新时，关于菜单显示小红点且按钮切换为展示更新弹窗。
    systemStore.updateInfo = {
      currentVersion: '1.1.5',
      latestVersion: '1.2.0',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.2.0',
      canSelfUpdate: true,
      assetName: 'tunnelkit.dmg',
      assetUrl: 'https://github.com/maybk/tunnelkit/releases/download/v1.2.0/tunnelkit.dmg',
      assetSize: 1024,
      assetSha256: 'abc',
    }
    await nextTick()
    expect(wrapper.find('[data-testid="settings-nav-about-dot"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="settings-about-update-dot"]').exists()).toBe(true)
    await wrapper.get('[data-testid="about-show-update"]').trigger('click')
    expect(systemStore.updateDialogOpen).toBe(true)

    // 系统信息尚未就绪时显示中性「检测中…」，不误报红色「未就绪」。
    systemStore.systemInfo = null
    await nextTick()
    expect(wrapper.get('[data-testid="about-helper-status"]').text()).toBe('检测中…')
  })
})
