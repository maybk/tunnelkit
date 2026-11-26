import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it } from 'vite-plus/test'
import AppSidebar from '@/layouts/AppSidebar.vue'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

describe('desktop sidebar presentation contract', () => {
  it('places create action in the list header and settings entry with update dot in the footer', async () => {
    const wrapper = mount(AppSidebar, {
      props: {
        profiles: [createTestProfile()],
        selectedId: 'profile-1',
        vpnState: createTestVpnState(),
        platform: 'macos',
        hasUpdate: false,
      },
    })
    expect(wrapper.text()).not.toContain('TunnelKit')
    const newBtn = wrapper.get('[data-testid="sidebar-new-profile-btn"]')
    expect(newBtn.attributes('aria-label')).toBe('新建连接')
    await newBtn.trigger('click')
    expect(wrapper.emitted('createProfile')).toHaveLength(1)

    const settingsBtn = wrapper.get('[data-testid="sidebar-settings-btn"]')
    expect(settingsBtn.text()).toContain('设置')
    expect(wrapper.find('[data-testid="sidebar-settings-update-dot"]').exists()).toBe(false)
    await settingsBtn.trigger('click')
    expect(wrapper.emitted('openSettings')).toHaveLength(1)

    await wrapper.setProps({
      hasUpdate: true,
      vpnState: createTestVpnState({
        status: 'disconnected',
        profileId: 'profile-1',
        error: { message: '认证失败', hint: '', trustedCert: '' },
      }),
    })
    expect(wrapper.find('[data-testid="sidebar-settings-update-dot"]').exists()).toBe(true)
    expect(wrapper.find('span.bg-tone-danger-solid').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders empty state, keeps menu anchor highlighted, and opens delete confirm dialog', async () => {
    const emptyWrapper = mount(AppSidebar, {
      props: {
        profiles: [],
        selectedId: '',
        vpnState: createTestVpnState(),
      },
    })
    expect(emptyWrapper.text()).toContain('暂无连接')
    emptyWrapper.unmount()

    const profile = createTestProfile()
    const wrapper = mount(AppSidebar, {
      props: {
        profiles: [profile],
        selectedId: profile.id,
        vpnState: createTestVpnState(),
      },
      attachTo: document.body,
    })

    await wrapper.get(`[data-testid="sidebar-profile-${profile.id}"]`).trigger('contextmenu')
    await nextTick()

    expect(wrapper.find('[data-menu-open="true"]').exists()).toBe(true)
    const menuItems = [...document.querySelectorAll('[role="menuitem"]')].map((el) =>
      el.textContent?.trim(),
    )
    expect(menuItems).toEqual(['连接', '删除'])

    document
      .querySelectorAll('[role="menuitem"]')[1]
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    const dialog = document.querySelector('[data-slot="alert-dialog-content"]')!
    expect(dialog).not.toBeNull()
    expect(dialog.textContent).toContain('删除连接')
    expect(dialog.textContent).toContain(
      `删除后将永久移除连接「${profile.name}」及其已保存的密码与连接日志，此操作无法撤销。`,
    )
    wrapper.unmount()
  })
})
