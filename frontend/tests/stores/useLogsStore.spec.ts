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

  it('coalesces concurrent refreshes and fetches the latest failure log after an in-flight read completes', async () => {
    const resolvers: Array<(snapshot: LogViewSnapshot) => void> = []
    const getLog = vi.spyOn(vpnApi, 'getLog').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolvers.push(resolve)
        }),
    )
    const store = useLogsStore()

    // First read starts right after ResetFile (empty snapshot)
    const first = store.fetchLog('p2', true)
    // Pre-launch failure occurs while first read is still in-flight
    await store.fetchLog('p2', true)
    await store.fetchLog('p2', true)
    expect(getLog).toHaveBeenCalledTimes(1)

    resolvers[0]?.({ profileId: 'p2', content: '', exists: true, updatedAt: 't1' })
    await vi.waitFor(() => {
      expect(getLog).toHaveBeenCalledTimes(2)
    })
    resolvers[1]?.({
      profileId: 'p2',
      content: '2026-11-12 10:40:01 ERROR 启动连接失败: 辅助组件授权配置未完成',
      exists: true,
      updatedAt: 't2',
    })
    await first

    expect(store.contentForProfile('p2')).toContain('ERROR 启动连接失败')
    expect(store.updatedAtForProfile('p2')).toBe('t2')
    expect(store.loading).toBe(false)
  })
})
