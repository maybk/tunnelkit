import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { VpnState } from '@/api/vpn'

const api = {
  getVpnState: vi.fn<() => Promise<VpnState>>(),
  connectVpn: vi.fn<(id: string) => Promise<void>>(),
  cancelConnect: vi.fn<() => Promise<void>>(),
  disconnectVpn: vi.fn<() => Promise<void>>(),
  getVpnLog: vi.fn(),
  clearVpnLog: vi.fn(),
}

let emitState: ((event: { data: unknown }) => void) | null = null

vi.mock('@/api/vpn', () => ({
  VPN_STATE_EVENT: 'vpn:state',
  toVpnState: (raw: unknown) => raw,
  ...api,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((_name: string, handler: (event: { data: unknown }) => void) => {
      emitState = handler
      return () => {
        emitState = null
      }
    }),
  },
}))

// createGlobalState 是模块级单例，每个用例重新加载模块以获得干净状态。
const loadStores = async () => {
  vi.resetModules()
  const { useVpn } = await import('@/stores/useVpn')
  const { useNotice } = await import('@/stores/useNotice')
  return { vpn: useVpn(), notice: useNotice() }
}

describe('useVpn', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    emitState = null
  })

  it('applies pushed state events', async () => {
    const { vpn } = await loadStores()
    vpn.startWatching()
    emitState?.({
      data: { status: 'connected', profileId: 'p1', connectedAt: '2026-10-19T12:00:00Z' },
    })
    expect(vpn.state.value).toMatchObject({ status: 'connected', profileId: 'p1' })
    vpn.stopWatching()
    expect(emitState).toBeNull()
  })

  it('keeps errors that the backend already put into state out of the global notice', async () => {
    const { vpn, notice } = await loadStores()
    api.connectVpn.mockRejectedValueOnce(new Error('网关证书未受信任'))
    api.getVpnState.mockResolvedValueOnce({
      status: 'disconnected',
      profileId: 'p1',
      error: { message: '网关证书未受信任', trustedCert: 'a'.repeat(64) },
    })

    await vpn.connect('p1')

    expect(api.connectVpn).toHaveBeenCalledWith('p1')
    expect(vpn.pending.value).toBe(false)
    expect(vpn.state.value.error?.trustedCert).toBe('a'.repeat(64))
    expect(notice.message.value).toBe('')
  })

  it('shows rejected operations that are not reflected in state', async () => {
    const { vpn, notice } = await loadStores()
    api.connectVpn.mockRejectedValueOnce(new Error('已有活动连接，请先断开后再连接'))
    api.getVpnState.mockResolvedValueOnce({ status: 'connected', profileId: 'p2' })

    await vpn.connect('p1')

    expect(notice.message.value).toBe('已有活动连接，请先断开后再连接')
  })

  it('ignores repeated clicks on the same profile while an operation is pending, but allows switching to another profile', async () => {
    const { vpn } = await loadStores()
    let resolveFirst!: () => void
    let resolveSecond!: () => void
    api.connectVpn
      .mockReturnValueOnce(new Promise<void>((r) => (resolveFirst = r)))
      .mockReturnValueOnce(new Promise<void>((r) => (resolveSecond = r)))

    const first = vpn.connect('p1')
    await vpn.connect('p1')
    expect(api.connectVpn).toHaveBeenCalledTimes(1)

    const second = vpn.connect('p2')
    expect(api.connectVpn).toHaveBeenCalledTimes(2)
    expect(api.connectVpn).toHaveBeenLastCalledWith('p2')

    resolveFirst()
    resolveSecond()
    await Promise.all([first, second])
    expect(vpn.pending.value).toBe(false)
  })

  it('forwards cancel and surfaces log errors', async () => {
    const { vpn, notice } = await loadStores()
    api.cancelConnect.mockResolvedValueOnce()
    await vpn.cancel()
    expect(api.cancelConnect).toHaveBeenCalledTimes(1)

    api.getVpnLog.mockRejectedValueOnce(new Error('配置 ID 无效'))
    await vpn.loadLog('bad')
    expect(vpn.logLoading.value).toBe(false)
    expect(notice.message.value).toBe('配置 ID 无效')
  })
})
