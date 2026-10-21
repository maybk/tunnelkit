import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Profile } from '@/api/profile'
import type { VpnState } from '@/api/vpn'
import ConnectionHeader from '@/components/connection/ConnectionHeader.vue'

const profile: Profile = {
  id: 'p1',
  name: '公司 VPN',
  host: 'vpn.example.com',
  port: 443,
  username: 'alice',
  trustedCert: '',
  auth: 'password',
  routes: 'gateway',
  dns: 'split',
  reconnect: true,
  hasPassword: true,
  updatedAt: '',
}

const mountHeader = (vpnState: VpnState, extra: Record<string, unknown> = {}) =>
  mount(ConnectionHeader, {
    props: { profile, vpnState, pending: false, nowMs: Date.now(), helperReady: true, ...extra },
  })

const button = (wrapper: ReturnType<typeof mountHeader>) => wrapper.get('button')

describe('ConnectionHeader', () => {
  it('offers connect when idle and emits the profile id', async () => {
    const wrapper = mountHeader({ status: 'disconnected' })
    expect(button(wrapper).text()).toBe('连接')
    expect(button(wrapper).classes()).toContain('text-primary-foreground')
    expect(button(wrapper).classes()).toContain('text-body')
    await button(wrapper).trigger('click')
    expect(wrapper.emitted('connect')?.[0]).toEqual(['p1'])
  })

  it('offers cancel while connecting', async () => {
    const wrapper = mountHeader({ status: 'connecting', profileId: 'p1' })
    expect(button(wrapper).text()).toContain('取消连接')
    await button(wrapper).trigger('click')
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })

  it('shows a disabled disconnecting state', () => {
    const wrapper = mountHeader({ status: 'disconnecting', profileId: 'p1' })
    expect(button(wrapper).text()).toContain('正在断开')
    expect(button(wrapper).attributes('disabled')).toBeDefined()
  })

  it('offers disconnect when connected', async () => {
    const wrapper = mountHeader({
      status: 'connected',
      profileId: 'p1',
      connectedAt: new Date().toISOString(),
    })
    expect(button(wrapper).text()).toBe('断开连接')
    expect(button(wrapper).classes()).toContain('text-destructive')
    await button(wrapper).trigger('click')
    expect(wrapper.emitted('disconnect')).toHaveLength(1)
  })

  it('offers one-click switch while another profile is active, and blocks when disconnecting or helper is not ready', async () => {
    const switchable = mountHeader({ status: 'connected', profileId: 'p2', profileName: '家里' })
    expect(button(switchable).attributes('disabled')).toBeUndefined()
    expect(button(switchable).text()).toBe('切换连接')
    expect(switchable.text()).toContain('「家里」正在使用中')
    await button(switchable).trigger('click')
    expect(switchable.emitted('connect')?.[0]).toEqual(['p1'])

    const otherDisconnecting = mountHeader({
      status: 'disconnecting',
      profileId: 'p2',
      profileName: '家里',
    })
    expect(button(otherDisconnecting).attributes('disabled')).toBeDefined()
    expect(button(otherDisconnecting).attributes('title')).toBe('正在断开当前活动的连接，请稍候')

    const noHelper = mountHeader({ status: 'disconnected' }, { helperReady: false })
    expect(button(noHelper).attributes('disabled')).toBeDefined()
    expect(button(noHelper).attributes('title')).toBe('特权辅助组件 tunnelkit-helper 尚未就绪')
  })
})
