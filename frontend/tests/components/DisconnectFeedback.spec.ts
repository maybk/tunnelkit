import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { vpnApi } from '@/api/vpn'
import { useVpnStore } from '@/stores/useVpnStore'
import VpnConnectionHero from '@/components/vpn/VpnConnectionHero.vue'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'
import type { VpnState } from '@/types/vpn'

afterEach(() => vi.useRealTimers())

describe('fast disconnect feedback', () => {
  it('clears immediate disconnect feedback when a request fails without changing network state', async () => {
    const store = useVpnStore()
    store.state = createTestVpnState({
      status: 'connected',
      phase: 'connected',
      profileId: 'profile-1',
    })
    vi.spyOn(vpnApi, 'disconnect').mockRejectedValue(new Error('断开失败'))
    const request = store.disconnect()
    expect(store.pendingAction?.kind).toBe('disconnect')
    await request
    expect(store.pendingAction).toBeNull()
    expect(store.busyAction).toBe(false)
    expect(store.state.status).toBe('connected')
  })

  it('does not relight a connect spinner after a connected state precedes request completion', async () => {
    const store = useVpnStore()
    let receive: (state: VpnState) => void = () => {}
    vi.spyOn(vpnApi, 'subscribeState').mockImplementation((listener) => {
      receive = listener
      return () => {}
    })
    let finish = () => {}
    vi.spyOn(vpnApi, 'connect').mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve
        }),
    )
    store.startWatching()
    const request = store.requestConnect('profile-1')
    expect(store.pendingAction?.kind).toBe('connect')
    receive(
      createTestVpnState({ profileId: 'profile-1', status: 'connecting', phase: 'connecting' }),
    )
    expect(store.pendingAction).toBeNull()
    receive(createTestVpnState({ profileId: 'profile-1', status: 'connected', phase: 'connected' }))
    expect(store.busyAction).toBe(true)
    expect(store.pendingAction).toBeNull()
    finish()
    await request
    store.stopWatching()
  })

  it('keeps a short disconnect animation then settles without restarting a connect spinner', async () => {
    vi.useFakeTimers()
    const store = useVpnStore()
    let receive: (state: VpnState) => void = () => {}
    vi.spyOn(vpnApi, 'subscribeState').mockImplementation((listener) => {
      receive = listener
      return () => {}
    })
    let finish = () => {}
    vi.spyOn(vpnApi, 'disconnect').mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve
        }),
    )
    store.startWatching()
    receive(createTestVpnState({ status: 'connected', phase: 'connected', profileId: 'profile-1' }))
    const Host = defineComponent({
      setup: () => () =>
        h(VpnConnectionHero, {
          profile: createTestProfile(),
          state: store.state,
          busy: store.busyAction,
          pendingAction: store.pendingAction,
          elapsedText: '',
        }),
    })
    const wrapper = mount(Host)
    const button = wrapper.get('[data-testid="vpn-connect-toggle"]')
    const disconnecting = store.disconnect()
    await nextTick()
    expect(button.text()).toBe('正在断开…')
    receive(
      createTestVpnState({
        status: 'disconnecting',
        phase: 'disconnecting',
        profileId: 'profile-1',
      }),
    )
    await nextTick()
    receive(createTestVpnState({ profileId: 'profile-1' }))
    await nextTick()
    expect(store.busyAction).toBe(true)
    expect(button.text()).toBe('正在断开…')
    vi.advanceTimersByTime(350)
    await nextTick()
    expect(button.text()).toBe('连接')
    expect(button.attributes('aria-busy')).toBe('false')
    expect(button.find('.animate-spin').exists()).toBe(false)
    finish()
    await disconnecting
    await nextTick()
    expect(button.text()).toBe('连接')
    expect(wrapper.get('[data-testid="vpn-connect-toggle"]').element).toBe(button.element)
    wrapper.unmount()
    store.stopWatching()
  })
})
