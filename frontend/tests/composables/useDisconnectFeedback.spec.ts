import { effectScope, reactive } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { useDisconnectFeedback } from '@/composables/useDisconnectFeedback'
import { createTestVpnState } from '../fixtures/profiles'
import type { VpnPendingAction } from '@/types/vpn'

afterEach(() => vi.useRealTimers())

function setup() {
  vi.useFakeTimers()
  const source = reactive({
    profileId: 'p1',
    pendingAction: null as VpnPendingAction | null,
    state: createTestVpnState({ profileId: 'p1', status: 'connected' }),
    busy: false,
  })
  const scope = effectScope()
  const feedback = scope.run(() => useDisconnectFeedback(source))!
  const start = () => {
    source.busy = true
    source.pendingAction = { kind: 'disconnect', profileId: 'p1' }
  }
  return { source, scope, feedback, start }
}

describe('minimum disconnect button feedback', () => {
  it('retains feedback for 350ms without holding back the disconnected state', () => {
    const { source, scope, feedback, start } = setup()
    start()
    source.state = createTestVpnState({ profileId: 'p1' })
    source.pendingAction = null
    source.busy = false
    expect(source.state.status).toBe('disconnected')
    expect(feedback.value).toBe(true)
    vi.advanceTimersByTime(349)
    expect(feedback.value).toBe(true)
    vi.advanceTimersByTime(1)
    expect(feedback.value).toBe(false)
    scope.stop()
  })

  it('does not add another wait after a slow operation and cancels feedback on failure', () => {
    const { source, scope, feedback, start } = setup()
    start()
    source.state.status = 'disconnecting'
    source.pendingAction = null
    vi.advanceTimersByTime(500)
    expect(feedback.value).toBe(false)
    source.state.status = 'disconnected'
    source.busy = false
    expect(feedback.value).toBe(false)
    source.state.status = 'connected'
    start()
    source.pendingAction = null
    source.busy = false
    expect(feedback.value).toBe(false)
    scope.stop()
  })

  it('clears pending feedback and timers when switching profiles or disposing', () => {
    const { source, scope, feedback, start } = setup()
    start()
    source.profileId = 'p2'
    expect(feedback.value).toBe(false)
    source.profileId = 'p1'
    source.pendingAction = null
    start()
    expect(feedback.value).toBe(true)
    scope.stop()
    expect(vi.getTimerCount()).toBe(0)
  })
})
