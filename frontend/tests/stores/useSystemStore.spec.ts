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

  it('reuses an in-flight silent check when manually checking updates and reopens dismissed updates', async () => {
    let resolveCheck!: (info: Awaited<ReturnType<typeof systemApi.checkUpdate>>) => void
    const checkSpy = vi.spyOn(systemApi, 'checkUpdate').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCheck = resolve
        }),
    )

    const store = useSystemStore()
    store.dismissUpdate('1.2.0')

    const silentPromise = store.checkUpdate({ silent: true })
    const explicitPromise = store.checkUpdate({ silent: false })
    expect(checkSpy).toHaveBeenCalledTimes(1)

    resolveCheck({
      currentVersion: '1.1.0',
      latestVersion: '1.2.0',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.2.0',
      canSelfUpdate: true,
      assetName: 'tunnelkit-darwin-arm64.dmg',
      assetUrl: 'https://github.com/maybk/tunnelkit/releases/download/v1.2.0/tunnelkit.dmg',
      assetSize: 4096,
      assetSha256: 'def456',
    })

    const [silentInfo, explicitInfo] = await Promise.all([silentPromise, explicitPromise])
    expect(silentInfo?.latestVersion).toBe('1.2.0')
    expect(explicitInfo?.latestVersion).toBe('1.2.0')
    expect(store.updateDialogOpen).toBe(true)
  })

  it('downgrades elevation cancellation to warning during applyUpdate and guards dismiss while applying', async () => {
    let rejectApply!: (reason?: unknown) => void
    const subscribeSpy = vi.spyOn(systemApi, 'subscribeUpdateProgress').mockReturnValue(() => {})
    vi.spyOn(systemApi, 'applyUpdate').mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectApply = reject
        }),
    )

    const store = useSystemStore()
    const noticeStore = useNoticeStore()
    store.updateInfo = {
      currentVersion: '1.1.0',
      latestVersion: '1.2.0',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v1.2.0',
      canSelfUpdate: true,
      assetName: 'tunnelkit-darwin-arm64.dmg',
      assetUrl: 'https://github.com/maybk/tunnelkit/releases/download/v1.2.0/tunnelkit.dmg',
      assetSize: 8192,
      assetSha256: 'def456',
    }
    store.updateDialogOpen = true

    const applyPromise = store.applyUpdate()
    expect(subscribeSpy).toHaveBeenCalledTimes(1)
    expect(store.isApplyingUpdate).toBe(true)
    expect(store.updateProgress?.totalBytes).toBe(8192)

    store.dismissUpdate('1.2.0')
    expect(store.updateDialogOpen).toBe(true)

    rejectApply(new Error('已取消管理员授权，更新未安装'))
    await applyPromise

    expect(store.isApplyingUpdate).toBe(false)
    expect(store.updateProgress).toBeNull()
    expect(store.updateDialogOpen).toBe(true)
    expect(noticeStore.items).toHaveLength(1)
    expect(noticeStore.items[0]).toMatchObject({
      tone: 'warning',
      title: '已取消管理员授权',
      message: '更新未安装',
    })
  })

  it('prevents duplicate release page launches and closes update dialog on open', async () => {
    let resolveOpen!: () => void
    const openSpy = vi.spyOn(systemApi, 'openExternalUrl').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveOpen = resolve
        }),
    )

    const store = useSystemStore()
    store.updateDialogOpen = true

    const first = store.openReleasePage('https://github.com/maybk/tunnelkit/releases/tag/v1.2.0')
    const second = store.openReleasePage('https://github.com/maybk/tunnelkit/releases/tag/v1.2.0')
    expect(store.isOpeningRelease).toBe(true)
    expect(openSpy).toHaveBeenCalledTimes(1)

    resolveOpen()
    await Promise.all([first, second])

    expect(store.isOpeningRelease).toBe(false)
    expect(store.updateDialogOpen).toBe(false)
  })
})
