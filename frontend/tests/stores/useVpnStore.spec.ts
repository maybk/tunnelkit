import { describe, expect, it, vi } from 'vite-plus/test'
import { vpnApi } from '@/api/vpn'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useVpnStore } from '@/stores/useVpnStore'
import type { VpnState } from '@/types/vpn'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

describe('VPN challenge lifecycle', () => {
  it.each(['connect', 'disconnect', 'cancel'] as const)(
    'reports a new %s failure when only a stale inline error remains',
    async (action) => {
      const profiles = useProfilesStore()
      profiles.profiles = [createTestProfile()]
      profiles.selectProfile('profile-1')
      const notices = useNoticeStore()
      const showError = vi.spyOn(notices, 'showError').mockImplementation(() => 0)
      const store = useVpnStore()
      const stale = createTestVpnState({
        revision: 7,
        profileId: 'profile-1',
        error: { message: '上一次认证失败', hint: '', trustedCert: '' },
      })
      const failure = new Error('本次宿主调用失败')
      vi.spyOn(vpnApi, action).mockRejectedValue(failure)
      const read = vi.spyOn(vpnApi, 'getState')
      for (const sync of ['failed', 'unchanged', 'older', 'empty'] as const) {
        store.state = { ...stale }
        showError.mockClear()
        if (sync === 'failed') read.mockRejectedValueOnce(new Error('本次状态读取失败'))
        else
          read.mockResolvedValueOnce(
            sync === 'empty'
              ? createTestVpnState()
              : { ...stale, revision: sync === 'older' ? 6 : 7 },
          )
        if (action === 'connect') await store.requestConnect('profile-1')
        else await store[action]()
        expect(showError).toHaveBeenCalledOnce()
        expect(showError.mock.lastCall?.[0]).toBe(failure)
        expect(store.state.error?.message).toBe(stale.error?.message)
        expect(store.busyAction).toBe(false)
        expect(store.cancelingConnect).toBe(false)
        expect(store.pendingAction).toBeNull()
      }
    },
  )

  it.each(['connect', 'disconnect', 'cancel'] as const)(
    'keeps a newer %s error event visible without a duplicate toast when the query fails',
    async (action) => {
      const profiles = useProfilesStore()
      profiles.profiles = [createTestProfile()]
      profiles.selectProfile('profile-1')
      const store = useVpnStore()
      store.state = createTestVpnState({ revision: 7, profileId: 'profile-1' })
      const notices = useNoticeStore()
      const showError = vi.spyOn(notices, 'showError').mockImplementation(() => 0)
      let emitState!: (state: VpnState) => void
      vi.spyOn(vpnApi, 'subscribeState').mockImplementation((listener) => {
        emitState = listener
        return () => {}
      })
      store.startWatching()
      vi.spyOn(vpnApi, 'getState').mockRejectedValueOnce(new Error('状态读取失败'))
      vi.spyOn(vpnApi, action).mockImplementationOnce(async () => {
        emitState(
          createTestVpnState({
            revision: 8,
            profileId: 'profile-1',
            error: { message: '本次清理失败', hint: '', trustedCert: '' },
          }),
        )
        throw new Error('本次清理失败')
      })
      try {
        if (action === 'connect') await store.requestConnect('profile-1')
        else await store[action]()
        expect(store.state.error?.message).toBe('本次清理失败')
        expect(showError).not.toHaveBeenCalled()
        expect(store.busyAction).toBe(false)
        expect(store.cancelingConnect).toBe(false)
      } finally {
        store.stopWatching()
      }
    },
  )

  it('does not restore an older query after a newer state event', async () => {
    let resolveRead!: (state: VpnState) => void
    let emitState!: (state: VpnState) => void
    vi.spyOn(vpnApi, 'getState').mockImplementation(
      () => new Promise((resolve) => (resolveRead = resolve)),
    )
    vi.spyOn(vpnApi, 'subscribeState').mockImplementation((listener) => {
      emitState = listener
      return () => {}
    })
    const store = useVpnStore()
    store.startWatching()
    const reading = store.loadState()
    emitState(createTestVpnState({ revision: 3, status: 'connected', phase: 'connected' }))
    resolveRead(
      createTestVpnState({
        revision: 2,
        status: 'connecting',
        phase: 'waiting_input',
        prompt: { id: 'old', kind: 'otp', message: 'Synthetic prompt', url: '' },
      }),
    )
    await reading
    expect(store.state.status).toBe('connected')
    expect(store.state.prompt).toBeNull()
    store.stopWatching()
  })

  it('cancels the specific challenge and refreshes authoritative state', async () => {
    const answer = vi.spyOn(vpnApi, 'answerPrompt').mockResolvedValue()
    const cancel = vi.spyOn(vpnApi, 'cancel').mockResolvedValue()
    vi.spyOn(vpnApi, 'getState').mockResolvedValue(createTestVpnState({ revision: 2 }))
    const store = useVpnStore()
    store.state = createTestVpnState({
      revision: 1,
      status: 'connecting',
      phase: 'waiting_input',
      prompt: { id: 'active', kind: 'otp', message: 'Synthetic prompt', url: '' },
    })
    await store.answerChallenge('', true)
    expect(answer).toHaveBeenCalledWith('active', '', true)
    expect(cancel).not.toHaveBeenCalled()
    expect(store.state.prompt).toBeNull()
  })

  it('does not attribute an old response error or completion to a new challenge', async () => {
    let rejectOld!: (reason: Error) => void
    let finishNew!: () => void
    vi.spyOn(vpnApi, 'answerPrompt')
      .mockImplementationOnce(() => new Promise((_, reject) => (rejectOld = reject)))
      .mockImplementationOnce(() => new Promise((resolve) => (finishNew = resolve)))
    const store = useVpnStore()
    const waiting = (id: string, revision: number) =>
      createTestVpnState({
        revision,
        status: 'connecting',
        phase: 'waiting_input',
        prompt: { id, kind: 'otp', message: 'Synthetic prompt', url: '' },
      })
    vi.spyOn(vpnApi, 'getState').mockImplementation(async () => store.state)
    store.state = waiting('first', 1)
    const oldRequest = store.answerChallenge('synthetic')
    store.state = waiting('second', 2)
    const newRequest = store.answerChallenge('synthetic')
    rejectOld(new Error('old request failed'))
    await oldRequest
    expect(store.answeringPrompt).toBe(true)
    expect(store.promptError).toBe('')
    finishNew()
    await newRequest
  })

  it('ignores a click emitted by a superseded challenge', async () => {
    const answer = vi.spyOn(vpnApi, 'answerPrompt')
    const store = useVpnStore()
    store.state = createTestVpnState({
      status: 'connecting',
      phase: 'waiting_input',
      prompt: { id: 'new', kind: 'otp', message: '', url: '' },
    })
    await store.answerChallenge('', true, 'old')
    expect(answer).not.toHaveBeenCalled()
  })

  it('does not resend an accepted answer while waiting for the next phase', async () => {
    const answer = vi.spyOn(vpnApi, 'answerPrompt').mockResolvedValue()
    const store = useVpnStore()
    store.state = createTestVpnState({
      status: 'connecting',
      phase: 'waiting_input',
      prompt: { id: 'active', kind: 'otp', message: '', url: '' },
    })
    vi.spyOn(vpnApi, 'getState').mockImplementation(async () => store.state)
    await store.answerChallenge('synthetic')
    await store.answerChallenge('synthetic')
    expect(answer).toHaveBeenCalledOnce()
    expect(store.answeringPrompt).toBe(true)
  })

  it('syncs backend error state on connect/disconnect failure without duplicating inline error toasts', async () => {
    const profiles = useProfilesStore()
    profiles.profiles = [
      createTestProfile({ id: 'profile-1', name: '主网关' }),
      createTestProfile({ id: 'profile-2', name: '备用网关' }),
    ]
    profiles.selectProfile('profile-1')

    const notices = useNoticeStore()
    const showError = vi.spyOn(notices, 'showError')
    const store = useVpnStore()

    vi.spyOn(vpnApi, 'connect').mockRejectedValueOnce(new Error('用户名或密码错误'))
    vi.spyOn(vpnApi, 'getState').mockResolvedValueOnce(
      createTestVpnState({
        revision: 2,
        status: 'disconnected',
        profileId: 'profile-1',
        error: { message: '用户名或密码错误', hint: '请检查凭据', trustedCert: '' },
      }),
    )
    await store.requestConnect('profile-1')
    expect(store.state.error?.message).toBe('用户名或密码错误')
    expect(showError).not.toHaveBeenCalled()

    vi.spyOn(vpnApi, 'connect').mockRejectedValueOnce(new Error('备用网关不可达'))
    vi.spyOn(vpnApi, 'getState').mockResolvedValueOnce(
      createTestVpnState({
        revision: 3,
        status: 'disconnected',
        profileId: 'profile-2',
        error: { message: '备用网关不可达', hint: '', trustedCert: '' },
      }),
    )
    await store.requestConnect('profile-2')
    expect(showError).toHaveBeenCalledOnce()
  })
})
