import { describe, expect, it, vi } from 'vite-plus/test'
import { vpnApi } from '@/api/vpn'
import { useLogsStore } from '@/stores/useLogsStore'
import type { LogViewSnapshot } from '@/types/log'

describe('log snapshot concurrency', () => {
  it('deduplicates in-flight reads and rejects a snapshot started before clearing', async () => {
    let finishRead: ((snapshot: LogViewSnapshot) => void) | undefined
    const getLog = vi.spyOn(vpnApi, 'getLog').mockImplementation(
      () =>
        new Promise((resolve) => {
          finishRead = resolve
        }),
    )
    vi.spyOn(vpnApi, 'clearLog').mockResolvedValue()
    const store = useLogsStore()
    const reading = store.fetchLog('p1')
    await store.fetchLog('p1')
    expect(getLog).toHaveBeenCalledOnce()
    await store.clearLog('p1')
    finishRead?.({ profileId: 'p1', content: 'stale log', exists: true, updatedAt: 'old' })
    await reading
    expect(store.contentForProfile('p1')).toBe('')
    expect(store.updatedAtForProfile('p1')).toBe('')
    expect(store.loading).toBe(false)
  })
})
