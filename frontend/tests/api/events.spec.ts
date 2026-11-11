import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { Call, Create, Events } from '@wailsio/runtime'
import { describe, expect, it, vi } from 'vite-plus/test'
import { EVENT_VPN_STATE, vpnApi } from '@/api/vpn'
import { EVENT_UPDATE_PROGRESS, systemApi } from '@/api/system'
import { useVpnStore } from '@/stores/useVpnStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import WorkspaceView from '@/views/WorkspaceView.vue'
import { createTestProfile } from '../fixtures/profiles'

describe('VPN state events', () => {
  it('keeps query and runtime-converted events identical without mutating event data', async () => {
    const listener = vi.fn()
    const unsubscribe = vpnApi.subscribeState(listener)
    const callback = vi.mocked(Events.On).mock.lastCall![1]

    for (const fields of [
      {},
      { error: null, info: null, stats: null },
      {
        status: 'connected',
        phase: 'connected',
        connectedAt: '2026-11-11T00:00:00Z',
        info: { interface: 'synthetic-tun', localIp: '192.0.2.10', routes: ['192.0.2.0/24'] },
        stats: { rxBytes: 1024, txBytes: 2048 },
      },
      {
        status: 'disconnected',
        phase: 'disconnected',
        error: {
          message: '配置系统网络失败',
          hint: '系统网络配置或恢复未完成，恢复记录会保留以便重试；请在「日志」中查看详细原因。',
        },
      },
      { phase: 'waiting_input', prompt: { id: 'synthetic', kind: 'otp', message: '请输入验证码' } },
    ]) {
      const raw = {
        revision: 1,
        status: 'connecting',
        phase: 'connecting',
        profileId: 'profile-1',
        ...fields,
      }
      vi.mocked(Call.ByID).mockResolvedValueOnce(structuredClone(raw))
      const queried = await vpnApi.getState()
      const data = Create.Events[EVENT_VPN_STATE]!(structuredClone(raw))
      const snapshot = structuredClone(data)
      callback({ name: EVENT_VPN_STATE, data })
      const pushed = listener.mock.lastCall![0]
      expect(pushed).toEqual(queried)
      expect(data).toEqual(snapshot)
    }
    unsubscribe()
  })

  it('renders success, normal disconnection and real failures from the event stream', async () => {
    const profiles = useProfilesStore()
    profiles.profiles = [createTestProfile()]
    profiles.selectProfile('profile-1')
    const store = useVpnStore()
    store.startWatching()
    const callback = vi.mocked(Events.On).mock.lastCall![1]
    const wrapper = mount(WorkspaceView)
    let revision = 0
    const emit = async (status: string, error?: { message: string }) => {
      const raw = {
        revision: ++revision,
        status,
        phase: status,
        profileId: 'profile-1',
        ...(error && { error }),
      }
      callback({ name: EVENT_VPN_STATE, data: Create.Events[EVENT_VPN_STATE]!(raw) })
      await nextTick()
    }
    const hasErrorDot = () =>
      wrapper.get('[data-testid="tab-trigger-status"]').find('span.bg-tone-danger-solid').exists()
    try {
      await emit('connected')
      expect(wrapper.text()).toContain('已建立安全连接')
      expect(store.state.info).toBeNull()
      expect(store.state.stats).toBeNull()
      expect(hasErrorDot()).toBe(false)
      callback({
        name: EVENT_VPN_STATE,
        data: Create.Events[EVENT_VPN_STATE]!({
          revision: ++revision,
          status: 'connecting',
          phase: 'reconnecting',
          profileId: 'profile-1',
          error: { message: '连接已意外断开' },
        }),
      })
      await nextTick()
      expect(hasErrorDot()).toBe(false)
      expect(wrapper.find('[data-testid="vpn-error-banner"]').exists()).toBe(false)
      await emit('disconnected')
      expect(wrapper.text()).toContain('当前未连接')
      expect(wrapper.text()).not.toContain('上次连接未成功')
      expect(hasErrorDot()).toBe(false)
      await emit('disconnected', { message: '系统网络恢复未完成' })
      expect(hasErrorDot()).toBe(true)
      expect(wrapper.find('[data-testid="vpn-error-banner"]').text()).toContain(
        '系统网络恢复未完成',
      )
      expect(wrapper.text()).not.toContain('上次连接未成功')
      profiles.profiles.push(createTestProfile({ id: 'profile-2', name: '其他连接' }))
      profiles.selectProfile('profile-2')
      await nextTick()
      expect(hasErrorDot()).toBe(false)
      expect(wrapper.find('[data-testid="vpn-error-banner"]').exists()).toBe(false)
      profiles.selectProfile('profile-1')
      await emit('connected')
      expect(hasErrorDot()).toBe(false)
      expect(wrapper.find('[data-testid="vpn-error-banner"]').exists()).toBe(false)
      await emit('disconnected')
      expect(hasErrorDot()).toBe(false)
      expect(wrapper.text()).toContain('当前未连接')
    } finally {
      wrapper.unmount()
      store.stopWatching()
    }
  })

  it('maps an already converted update event without changing the shared model', () => {
    const listener = vi.fn()
    const unsubscribe = systemApi.subscribeUpdateProgress(listener)
    const callback = vi.mocked(Events.On).mock.lastCall![1]
    const data = Create.Events[EVENT_UPDATE_PROGRESS]!({ stage: 'downloading', percent: 25 })
    callback({ name: EVENT_UPDATE_PROGRESS, data })
    expect(listener).toHaveBeenCalledWith(
      expect.objectContaining({ stage: 'downloading', percent: 25 }),
    )
    unsubscribe()
  })
})
