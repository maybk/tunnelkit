import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { systemApi } from '@/api/system'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useSystemStore } from '@/stores/useSystemStore'

describe('useSystemStore', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('checks for updates, loads system info, and coordinates about/update dialogs', async () => {
    vi.spyOn(systemApi, 'getInfo').mockResolvedValue({
      appVersion: '0.3.2',
      platform: 'darwin',
      helperReady: true,
      helperPath: '/Library/PrivilegedHelperTools/tunnelkit-helper',
    })
    vi.spyOn(systemApi, 'checkUpdate').mockResolvedValue({
      currentVersion: '0.3.2',
      latestVersion: '0.4.0',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.4.0',
      canSelfUpdate: true,
      assetName: 'tunnelkit-darwin-arm64.tar.gz',
      assetUrl: 'https://github.com/maybk/tunnelkit/releases/download/v0.4.0/tunnelkit.tar.gz',
      assetSize: 2048,
      assetSha256: 'abc123',
    })

    const store = useSystemStore()
    const sysInfo = await store.loadSystemInfo()
    expect(sysInfo?.helperReady).toBe(true)

    store.openAboutDialog()
    const info = await store.checkUpdate({ silent: false })
    expect(info?.hasUpdate).toBe(true)
    expect(store.hasUpdate).toBe(true)
    expect(store.aboutDialogOpen).toBe(false)
    expect(store.updateDialogOpen).toBe(true)
  })

  it('uses in-place button confirmation without duplicate toast while AboutDialog is open, and falls back to toast when closed', async () => {
    vi.useFakeTimers()
    vi.spyOn(systemApi, 'checkUpdate').mockResolvedValue({
      currentVersion: '1.0.0',
      latestVersion: '1.0.0',
      hasUpdate: false,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.0.0',
      canSelfUpdate: false,
      assetName: '',
      assetUrl: '',
      assetSize: 0,
      assetSha256: '',
    })

    const store = useSystemStore()
    const noticeStore = useNoticeStore()

    store.openAboutDialog()
    await store.checkUpdate({ silent: false })

    expect(store.updateUpToDate).toBe(true)
    expect(noticeStore.items).toHaveLength(0)

    vi.advanceTimersByTime(2500)
    expect(store.updateUpToDate).toBe(false)

    // Closing AboutDialog before timer expiry also clears transient button feedback immediately.
    await store.checkUpdate({ silent: false })
    expect(store.updateUpToDate).toBe(true)
    store.aboutDialogOpen = false
    expect(store.updateUpToDate).toBe(false)

    // When AboutDialog is closed before checkUpdate resolves, global toast serves as fallback.
    await store.checkUpdate({ silent: false })
    expect(store.updateUpToDate).toBe(false)
    expect(noticeStore.items).toHaveLength(1)
    expect(noticeStore.items[0]).toMatchObject({
      tone: 'success',
      title: '当前已是最新版本',
      message: 'v1.0.0',
    })
  })

  it('prevents stacked AboutDialog and UpdateDialog when a silent startup check resolves mid-flight', async () => {
    let resolveCheck!: (info: Awaited<ReturnType<typeof systemApi.checkUpdate>>) => void
    vi.spyOn(systemApi, 'checkUpdate').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCheck = resolve
        }),
    )

    const store = useSystemStore()
    const pending = store.checkUpdate({ silent: true })
    store.openAboutDialog()
    expect(store.aboutDialogOpen).toBe(true)

    resolveCheck({
      currentVersion: '1.0.0',
      latestVersion: '1.1.0',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.1.0',
      canSelfUpdate: true,
      assetName: 'tunnelkit-darwin-arm64.tar.gz',
      assetUrl: 'https://github.com/maybk/tunnelkit/releases/download/v1.1.0/tunnelkit.tar.gz',
      assetSize: 2048,
      assetSha256: 'abc123',
    })
    await pending

    expect(store.aboutDialogOpen).toBe(false)
    expect(store.updateDialogOpen).toBe(true)
  })
})
