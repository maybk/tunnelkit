import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vite-plus/test'
import { useAppBootstrap } from '@/composables/useAppBootstrap'
import { useDesktopWindowStore } from '@/stores/useDesktopWindowStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useSystemStore } from '@/stores/useSystemStore'
import { useVpnStore } from '@/stores/useVpnStore'
import { createTestProfile } from '../fixtures/profiles'

describe('app bootstrap failure isolation', () => {
  it('loads saved profiles even if reading VPN state fails', async () => {
    vi.useFakeTimers()
    const vpn = useVpnStore()
    const profiles = useProfilesStore()
    const system = useSystemStore()
    vi.spyOn(useDesktopWindowStore(), 'startWatching').mockImplementation(() => {})
    vi.spyOn(vpn, 'loadState').mockRejectedValue(new Error('IPC unavailable'))
    vi.spyOn(system, 'loadSystemInfo').mockResolvedValue(null)
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
    vi.useRealTimers()
  })
})
