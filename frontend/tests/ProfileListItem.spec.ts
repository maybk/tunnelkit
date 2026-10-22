import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import ProfileListItem from '@/components/sidebar/ProfileListItem.vue'

const profile: Profile = {
  id: 'p1',
  name: '办公 VPN',
  host: '198.51.100.10',
  port: 10443,
  username: 'alice',
  trustedCert: '',
  auth: 'password',
  routes: 'gateway',
  dns: 'split',
  reconnect: true,
  hasPassword: true,
  updatedAt: '',
}

describe('ProfileListItem', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  const mountItem = (vpnState: VpnState, extra: Record<string, unknown> = {}) =>
    mount(ProfileListItem, {
      attachTo: document.body,
      props: {
        profile,
        selected: false,
        vpnState,
        ...extra,
      },
    })

  it('emits select when clicking the main item button, but not when clicking the menu trigger', async () => {
    const wrapper = mountItem({ status: 'disconnected' })

    await wrapper.get('[data-testid="profile-menu-trigger"]').trigger('click')
    expect(wrapper.emitted('select')).toBeUndefined()

    await wrapper.get('[data-testid="profile-select-button"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual(['p1'])
    wrapper.unmount()
  })

  it('opens context menu on right-click and supports connect and delete confirmation', async () => {
    const wrapper = mountItem({ status: 'disconnected' })

    await wrapper.trigger('contextmenu')
    await wrapper.vm.$nextTick()

    const connectItem = document.body.querySelector('[data-testid="menu-connect"]') as HTMLElement
    expect(connectItem).not.toBeNull()
    expect(connectItem.textContent).toContain('连接')
    connectItem.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('connect')?.[0]).toEqual(['p1'])

    await wrapper.trigger('contextmenu')
    await wrapper.vm.$nextTick()
    const deleteItem = document.body.querySelector('[data-testid="menu-delete"]') as HTMLElement
    expect(deleteItem).not.toBeNull()
    expect(deleteItem.getAttribute('data-disabled')).toBeNull()
    deleteItem.click()
    await wrapper.vm.$nextTick()

    const confirmBtn = document.body.querySelector('[data-testid="confirm-delete"]') as HTMLElement
    expect(confirmBtn).not.toBeNull()
    confirmBtn.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('delete')?.[0]).toEqual(['p1'])
    wrapper.unmount()
  })

  it('disables delete and offers disconnect when the profile is connected', async () => {
    const wrapper = mountItem({
      status: 'connected',
      profileId: 'p1',
      connectedAt: new Date().toISOString(),
    })

    await wrapper.trigger('contextmenu')
    await wrapper.vm.$nextTick()

    const disconnectItem = document.body.querySelector(
      '[data-testid="menu-disconnect"]',
    ) as HTMLElement
    expect(disconnectItem).not.toBeNull()
    expect(disconnectItem.textContent).toContain('断开连接')

    const deleteItem = document.body.querySelector('[data-testid="menu-delete"]') as HTMLElement
    expect(deleteItem).not.toBeNull()
    expect(deleteItem.getAttribute('data-disabled')).not.toBeNull()

    disconnectItem.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('disconnect')).toHaveLength(1)
    wrapper.unmount()
  })
})
