import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { useAppBootstrap } from '@/composables/useAppBootstrap'
import { useDesktopWindowStore } from '@/stores/useDesktopWindowStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSettingsStore } from '@/stores/useSettingsStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { useVpnStore } from '@/stores/useVpnStore'
import { createTestProfile } from '../fixtures/profiles'

describe('app bootstrap failure isolation and auto-update gating', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads saved profiles even if reading VPN state fails and cancels timer on unmount', async () => {
    vi.useFakeTimers()
    const vpn = useVpnStore()
    const profiles = useProfilesStore()
    const settings = useSettingsStore()
    const system = useSystemStore()
    vi.spyOn(useDesktopWindowStore(), 'startWatching').mockImplementation(() => {})
    vi.spyOn(vpn, 'loadState').mockRejectedValue(new Error('IPC unavailable'))
    vi.spyOn(system, 'loadSystemInfo').mockResolvedValue(null)
    vi.spyOn(settings, 'loadSettings').mockResolvedValue({
      theme: 'system',
      autoCheckUpdate: true,
    })
    const loadProfiles = vi.spyOn(profiles, 'loadProfiles').mockImplementation(async () => {
      profiles.profiles = [createTestProfile()]
      profiles.selectProfile('profile-1')
      return profiles.profiles
    })
    const update = vi.spyOn(system, 'checkUpdate').mockResolvedValue(null)
    const wrapper = mount(defineComponent({ setup: useAppBootstrap, template: '<div />' }))
    await flushPromises()
    expect(loadProfiles).toHaveBeenCalledOnce()
    expect(profiles.selectedProfile?.name).toBe('总部网关')
    wrapper.unmount()
    await vi.runAllTimersAsync()
    expect(update).not.toHaveBeenCalled()
  })

  it('skips silent startup update check when autoCheckUpdate is disabled and runs it when enabled', async () => {
    vi.useFakeTimers()
    const vpn = useVpnStore()
    const profiles = useProfilesStore()
    const settings = useSettingsStore()
    const system = useSystemStore()
    vi.spyOn(useDesktopWindowStore(), 'startWatching').mockImplementation(() => {})
    vi.spyOn(vpn, 'loadState').mockResolvedValue(vpn.state)
    vi.spyOn(system, 'loadSystemInfo').mockResolvedValue(null)
    vi.spyOn(profiles, 'loadProfiles').mockResolvedValue([])
    const update = vi.spyOn(system, 'checkUpdate').mockResolvedValue(null)

    vi.spyOn(settings, 'loadSettings').mockImplementation(async () => {
      settings.settings = { theme: 'system', autoCheckUpdate: false }
      return settings.settings
    })
    const disabledWrapper = mount(defineComponent({ setup: useAppBootstrap, template: '<div />' }))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    expect(update).not.toHaveBeenCalled()
    disabledWrapper.unmount()

    vi.spyOn(settings, 'loadSettings').mockImplementation(async () => {
      settings.settings = { theme: 'dark', autoCheckUpdate: true }
      return settings.settings
    })
    const enabledWrapper = mount(defineComponent({ setup: useAppBootstrap, template: '<div />' }))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1500)
    expect(update).toHaveBeenCalledWith({ silent: true })
    enabledWrapper.unmount()
  })
})
