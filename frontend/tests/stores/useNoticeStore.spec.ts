import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vite-plus/test'
import { useNoticeStore } from '@/stores/useNoticeStore'

describe('useNoticeStore', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('deduplicates identical notices and resets auto-dismiss timer', () => {
    const store = useNoticeStore()
    const firstId = store.showSuccess('已复制脱敏日志到剪贴板')
    vi.advanceTimersByTime(3000)

    const secondId = store.showSuccess('已复制脱敏日志到剪贴板')
    expect(secondId).toBe(firstId)
    expect(store.items).toHaveLength(1)

    vi.advanceTimersByTime(1000)
    expect(store.items).toHaveLength(1)

    vi.advanceTimersByTime(2900)
    expect(store.items).toHaveLength(0)
  })

  it('replaces an existing notice in-place when replaceKey matches and promotes it to top', () => {
    const store = useNoticeStore()
    const idA = store.showInfo('正在检查更新…', undefined, { replaceKey: 'update-check' })
    store.showSuccess('已保存连接配置', '办公 VPN')

    const idA2 = store.showSuccess('已是最新版本', 'v1.1.2', { replaceKey: 'update-check' })
    expect(idA2).toBe(idA)
    expect(store.items).toHaveLength(2)
    expect(store.items.at(-1)).toMatchObject({
      id: idA,
      tone: 'success',
      title: '已是最新版本',
      message: 'v1.1.2',
      replaceKey: 'update-check',
    })
  })

  it('caps queue size and cleans up evicted timers', () => {
    const store = useNoticeStore()
    for (let i = 1; i <= 6; i++) {
      store.showInfo(`通知 ${i}`)
    }
    expect(store.items).toHaveLength(4)
    expect(store.items.map((item) => item.title)).toEqual(['通知 3', '通知 4', '通知 5', '通知 6'])

    store.clearAll()
    expect(store.items).toHaveLength(0)
  })
})
