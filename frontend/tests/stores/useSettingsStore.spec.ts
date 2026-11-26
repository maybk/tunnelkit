import { describe, expect, it, vi } from 'vite-plus/test'
import { profileApi } from '@/api/profile'
import { useAppearanceStore } from '@/stores/useAppearanceStore'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useSettingsStore } from '@/stores/useSettingsStore'

describe('useSettingsStore', () => {
  it('loads settings from backend and syncs appearance theme', async () => {
    vi.spyOn(profileApi, 'getSettings').mockResolvedValue({
      theme: 'dark',
      autoCheckUpdate: false,
    })

    const appearanceStore = useAppearanceStore()
    const settingsStore = useSettingsStore()
    const loaded = await settingsStore.loadSettings()

    expect(loaded).toEqual({ theme: 'dark', autoCheckUpdate: false })
    expect(settingsStore.loaded).toBe(true)
    expect(appearanceStore.theme).toBe('dark')
  })

  it('applies optimistic updates serially, reports toggle feedback, and rolls back on failure', async () => {
    const saveSpy = vi
      .spyOn(profileApi, 'saveSettings')
      .mockResolvedValueOnce({ theme: 'light', autoCheckUpdate: true })
      .mockResolvedValueOnce({ theme: 'light', autoCheckUpdate: false })
      .mockRejectedValueOnce(new Error('disk full'))

    const appearanceStore = useAppearanceStore()
    const noticeStore = useNoticeStore()
    const settingsStore = useSettingsStore()

    const themeOk = await settingsStore.saveSettings({ theme: 'light' })
    expect(themeOk).toBe(true)
    expect(settingsStore.settings.theme).toBe('light')
    expect(appearanceStore.theme).toBe('light')
    expect(noticeStore.items).toHaveLength(0)

    const toggleOk = await settingsStore.saveSettings({ autoCheckUpdate: false })
    expect(toggleOk).toBe(true)
    expect(settingsStore.settings.autoCheckUpdate).toBe(false)
    expect(noticeStore.items).toHaveLength(1)
    expect(noticeStore.items[0]).toMatchObject({
      tone: 'success',
      title: '已关闭自动检查更新',
      replaceKey: 'setting:autoCheckUpdate',
    })

    const failed = await settingsStore.saveSettings({ theme: 'dark' })
    expect(failed).toBe(false)
    expect(settingsStore.settings.theme).toBe('light')
    expect(appearanceStore.theme).toBe('light')
    expect(saveSpy).toHaveBeenCalledTimes(3)
    expect(noticeStore.items.some((item) => item.tone === 'danger')).toBe(true)
  })

  it('ignores a stale loadSettings response when a newer saveSettings occurred in flight', async () => {
    let resolveLoad!: (value: { theme: 'system'; autoCheckUpdate: true }) => void
    vi.spyOn(profileApi, 'getSettings').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveLoad = resolve
        }),
    )
    vi.spyOn(profileApi, 'saveSettings').mockResolvedValue({
      theme: 'dark',
      autoCheckUpdate: true,
    })

    const appearanceStore = useAppearanceStore()
    const settingsStore = useSettingsStore()

    const loadPromise = settingsStore.loadSettings()
    await settingsStore.saveSettings({ theme: 'dark' })
    expect(settingsStore.settings.theme).toBe('dark')

    resolveLoad({ theme: 'system', autoCheckUpdate: true })
    await loadPromise

    expect(settingsStore.settings.theme).toBe('dark')
    expect(appearanceStore.theme).toBe('dark')
  })
})
