import { describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import { vpnApi } from '@/api/vpn'
import { useProfilesStore } from '@/stores/useProfilesStore'
import { useVpnStore } from '@/stores/useVpnStore'
import { createTestProfile, createTestProfileInput, createTestVpnState } from '../fixtures/profiles'

describe('useProfilesStore and useVpnStore', () => {
  it('loads profiles, selects active profile, and saves/deletes profiles', async () => {
    const p1 = createTestProfile({ id: 'p1', name: '节点 1' })
    const p2 = createTestProfile({ id: 'p2', name: '节点 2' })
    let listItems = [p1, p2]
    vi.spyOn(profileApi, 'list').mockImplementation(async () => listItems)
    vi.spyOn(profileApi, 'save').mockResolvedValue(p2)
    vi.spyOn(profileApi, 'remove').mockImplementation(async (id: string) => {
      listItems = listItems.filter((item) => item.id !== id)
    })

    const store = useProfilesStore()
    await store.loadProfiles()
    expect(store.profiles).toHaveLength(2)
    expect(store.selectedId).toBe('p1')

    store.selectProfile('p2')
    expect(store.selectedProfile?.id).toBe('p2')

    await store.saveProfile(createTestProfileInput({ id: 'p2', name: '节点 2' }))
    expect(store.selectedId).toBe('p2')

    await store.deleteProfile('p2')
    expect(store.profiles).toHaveLength(1)
    expect(store.selectedId).toBe('p1')
  })

  it('loads and synchronizes VPN state in useVpnStore', async () => {
    const connectedState = createTestVpnState({
      status: 'connected',
      phase: 'connected',
      profileId: 'p1',
      profileName: '节点 1',
      connectedAt: '2026-04-01T12:00:00Z',
    })
    vi.spyOn(vpnApi, 'getState').mockResolvedValue(connectedState)

    const vpnStore = useVpnStore()
    await vpnStore.loadState()
    expect(vpnStore.state.status).toBe('connected')
    expect(vpnStore.state.profileId).toBe('p1')
    vpnStore.stopWatching()
  })

  it('allows cancelling a pending connect request', async () => {
    let finishConnect: (() => void) | undefined
    vi.spyOn(vpnApi, 'connect').mockImplementation(
      () => new Promise<void>((resolve) => (finishConnect = resolve)),
    )
    const cancel = vi.spyOn(vpnApi, 'cancel').mockResolvedValue()
    const vpnStore = useVpnStore()

    const connecting = vpnStore.requestConnect('p1')
    expect(vpnStore.busyAction).toBe(true)
    await vpnStore.cancel()

    expect(cancel).toHaveBeenCalledOnce()
    expect(vpnStore.cancelingConnect).toBe(false)
    finishConnect?.()
    await connecting
    expect(vpnStore.busyAction).toBe(false)
    vpnStore.stopWatching()
  })

  it('clears a failed prompt response and allows retry', async () => {
    const answer = vi
      .spyOn(vpnApi, 'answerPrompt')
      .mockRejectedValueOnce(new Error('temporary IPC failure'))
      .mockResolvedValueOnce()
    const vpnStore = useVpnStore()
    vpnStore.state = createTestVpnState({
      status: 'connecting',
      phase: 'waiting_input',
      profileId: 'p1',
      prompt: { id: 'challenge-1', kind: 'otp', message: 'OTP', url: '' },
    })
    vi.spyOn(vpnApi, 'getState').mockImplementation(async () => vpnStore.state)

    await vpnStore.answerChallenge('123456')
    expect(vpnStore.promptError).toContain('temporary IPC failure')
    expect(vpnStore.answeringPrompt).toBe(false)

    await vpnStore.answerChallenge('123456')
    expect(answer).toHaveBeenCalledTimes(2)
    expect(vpnStore.promptError).toBe('')
  })
})
