import { describe, expect, it, vi } from 'vite-plus/test'
import { vpnApi } from '@/api/vpn'
import { useVpnStore } from '@/stores/useVpnStore'
import type { VpnState } from '@/types/vpn'
import { createTestVpnState } from '../fixtures/profiles'

describe('VPN challenge lifecycle', () => {
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
})
