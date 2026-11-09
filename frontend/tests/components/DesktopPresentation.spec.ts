import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vite-plus/test'
import AppSidebar from '@/layouts/AppSidebar.vue'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

describe('original desktop presentation contract', () => {
  it('keeps the sidebar footer limited to the original create action', async () => {
    const wrapper = mount(AppSidebar, {
      props: {
        profiles: [createTestProfile()],
        selectedId: 'profile-1',
        vpnState: createTestVpnState(),
      },
    })
    expect(wrapper.text()).not.toContain('TunnelKit')
    expect(wrapper.get('[data-testid="sidebar-new-profile-btn"]').text()).toBe('新建连接')
    await wrapper.get('[data-testid="sidebar-new-profile-btn"]').trigger('click')
    expect(wrapper.emitted('createProfile')).toHaveLength(1)
    wrapper.unmount()
  })
})
