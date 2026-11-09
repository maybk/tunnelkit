import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vite-plus/test'
import VpnConnectionHero from '@/components/vpn/VpnConnectionHero.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import { getViewingStatusMeta } from '@/presentation/vpn'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

describe('VPN feedback continuity', () => {
  it('shows the current connection phase and pulse without leaking it to another profile', async () => {
    const state = createTestVpnState({
      status: 'connecting',
      phase: 'authenticating',
      profileId: 'profile-1',
    })
    const wrapper = mount(VpnConnectionHero, {
      props: { profile: createTestProfile(), state, elapsedText: '', busy: true },
    })
    expect(wrapper.text()).toContain('正在验证身份…')
    expect(wrapper.findComponent(StatusDot).props('pulse')).toBe(true)
    expect(wrapper.get('button').attributes('disabled')).toBeUndefined()
    await wrapper.setProps({ state: { ...state, phase: 'configuring' } })
    expect(wrapper.text()).toContain('正在配置网络…')
    expect(getViewingStatusMeta(state, 'another-profile')).toMatchObject({
      label: '未连接',
      pulse: false,
    })
    wrapper.unmount()
  })

  it('keeps failure feedback local to the affected profile and falls back for empty phases', () => {
    const failed = createTestVpnState({
      profileId: 'profile-1',
      error: { message: '失败', hint: '', trustedCert: '' },
    })
    expect(getViewingStatusMeta(failed, 'profile-1').tone).toBe('danger')
    expect(getViewingStatusMeta(failed, 'another-profile').tone).toBe('neutral')
    expect(
      getViewingStatusMeta({ ...failed, status: 'connecting', phase: '' }, 'profile-1').label,
    ).toBe('正在处理连接请求…')
  })

  it('shows immediate request feedback and keeps the same button across cancel and disconnect', async () => {
    const state = createTestVpnState()
    const wrapper = mount(VpnConnectionHero, {
      props: { profile: createTestProfile(), state, elapsedText: '' },
    })
    const button = wrapper.get('[data-testid="vpn-connect-toggle"]')
    await wrapper.setProps({
      busy: true,
      pendingAction: { kind: 'connect', profileId: 'profile-1' },
    })
    expect(button.find('.animate-spin').exists()).toBe(true)
    expect(button.attributes('aria-busy')).toBe('true')
    expect(button.attributes('disabled')).toBeDefined()
    await wrapper.setProps({
      state: { ...state, status: 'connecting', phase: 'connecting', profileId: 'profile-1' },
      pendingAction: null,
    })
    expect(button.attributes('disabled')).toBeUndefined()
    await wrapper.setProps({ canceling: true })
    expect(button.text()).toBe('正在取消…')
    expect(button.attributes('disabled')).toBeDefined()
    await wrapper.setProps({
      state: { ...state, status: 'disconnecting', phase: 'disconnecting', profileId: 'profile-1' },
      canceling: false,
    })
    expect(button.attributes('data-variant')).toBe('outline')
    expect(wrapper.get('[data-testid="vpn-connect-toggle"]').element).toBe(button.element)
    await wrapper.setProps({ busy: false, state })
    expect(button.text()).toBe('连接')
    expect(button.attributes('aria-busy')).toBe('false')
    expect(button.find('.animate-spin').exists()).toBe(false)
    wrapper.unmount()
  })
})
