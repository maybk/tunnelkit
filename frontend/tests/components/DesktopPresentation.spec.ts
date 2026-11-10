import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
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
