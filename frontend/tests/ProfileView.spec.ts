import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Profile, ProfileInput } from '@/api/profile'
import ProfileView from '@/views/ProfileView.vue'

vi.mock('@/api/profile', () => ({
  DEFAULT_VPN_PORT: 443,
  formatProfileText: vi.fn(async (input: ProfileInput) => `host = ${input.host}:${input.port}`),
  parseProfileText: vi.fn(async (base: ProfileInput) => ({ input: base, warnings: [] })),
}))

const profile: Profile = {
  id: 'p1',
  name: '办公 VPN',
  host: 'vpn.corp.local',
  port: 443,
  username: 'bob',
  hasPassword: true,
  trustedCert: '',
  auth: 'password',
  routes: 'gateway',
  dns: 'split',
  reconnect: true,
  updatedAt: '2026-10-21T10:00:00Z',
}

describe('ProfileView config tab actions', () => {
  it('hides save actions when clean and reveals reset and save buttons in the tab bar when dirty', async () => {
    const wrapper = mount(ProfileView, {
      props: {
        activeTab: 'config',
        profile,
        vpnState: { status: 'disconnected' },
        pending: false,
        nowMs: Date.now(),
        helperReady: true,
        saving: false,
        deleting: false,
        formError: '',
        logView: null,
        logLoading: false,
      },
    })

    expect(wrapper.find('[data-testid="tab-save-profile"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="tab-reset-profile"]').exists()).toBe(false)

    await wrapper.get('#profile-port').setValue('10443')
    const saveBtn = wrapper.get('[data-testid="tab-save-profile"]')
    const resetBtn = wrapper.get('[data-testid="tab-reset-profile"]')
    expect(saveBtn.text()).toBe('保存')
    expect(saveBtn.attributes('disabled')).toBeUndefined()

    await resetBtn.trigger('click')
    expect((wrapper.get('#profile-port').element as HTMLInputElement).value).toBe('443')
    expect(wrapper.find('[data-testid="tab-save-profile"]').exists()).toBe(false)
    expect(wrapper.emitted('resetFormError')).toHaveLength(1)

    await wrapper.get('#profile-port').setValue('8443')
    await wrapper.get('[data-testid="tab-save-profile"]').trigger('click')
    expect(wrapper.emitted('saveProfile')?.[0]?.[0]).toMatchObject({
      id: 'p1',
      port: 8443,
    })

    wrapper.unmount()
  })
})
