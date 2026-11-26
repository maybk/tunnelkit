import { afterEach, describe, expect, it, vi } from 'vite-plus/test'
import { systemApi } from '@/api/system'
import { useNoticeStore } from '@/stores/useNoticeStore'
import { useSystemStore } from '@/stores/useSystemStore'
import type { UpdateProgress } from '@/types/system'

describe('useSystemStore', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('checks for updates, loads system info, and opens the update dialog when an update is found', async () => {
    vi.spyOn(systemApi, 'getInfo').mockResolvedValue({
      appVersion: '0.3.2',
      platform: 'darwin',
      helperReady: true,
      helperPath: '/Library/PrivilegedHelperTools/tunnelkit-helper',
      dataDir: '/Users/alice/.tunnelkit',
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
    expect(sysInfo?.dataDir).toBe('/Users/alice/.tunnelkit')

    const info = await store.checkUpdate({ silent: false, inline: true })
    expect(info?.hasUpdate).toBe(true)
    expect(store.hasUpdate).toBe(true)
    expect(store.updateDialogOpen).toBe(true)
  })

  it('uses in-place button confirmation for inline checks without duplicate toast, and falls back to toast otherwise', async () => {
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

    await store.checkUpdate({ silent: false, inline: true })

    expect(store.updateUpToDate).toBe(true)
    expect(noticeStore.items).toHaveLength(0)

    vi.advanceTimersByTime(2500)
    expect(store.updateUpToDate).toBe(false)

    // Explicit clearUpToDateFeedback (e.g. leaving AboutView) clears transient feedback immediately.
    await store.checkUpdate({ silent: false, inline: true })
    expect(store.updateUpToDate).toBe(true)
    store.clearUpToDateFeedback()
    expect(store.updateUpToDate).toBe(false)

    // Non-inline manual check falls back to global toast notification.
    await store.checkUpdate({ silent: false })
    expect(store.updateUpToDate).toBe(false)
    expect(noticeStore.items).toHaveLength(1)
    expect(noticeStore.items[0]).toMatchObject({
      tone: 'success',
      title: '当前已是最新版本',
      message: 'v1.0.0',
    })
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
    const explicitPromise = store.checkUpdate({ silent: false, inline: true })
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

  it('handles two-stage download, defer, and preserves ready state when elevation is cancelled on install', async () => {
    let resolveDownload!: () => void
    let rejectInstall!: (reason?: unknown) => void
    const subscribeSpy = vi.spyOn(systemApi, 'subscribeUpdateProgress').mockReturnValue(() => {})
    vi.spyOn(systemApi, 'downloadUpdate').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveDownload = resolve
        }),
    )
    vi.spyOn(systemApi, 'installUpdate').mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectInstall = reject
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

    const downloadPromise = store.downloadUpdate()
    expect(subscribeSpy).toHaveBeenCalledTimes(1)
    expect(store.isDownloadingUpdate).toBe(true)
    expect(store.updatePhase).toBe('downloading')
    expect(store.updateProgress?.totalBytes).toBe(8192)

    store.dismissUpdate('1.2.0')
    expect(store.updateDialogOpen).toBe(true)

    resolveDownload()
    await downloadPromise

    expect(store.isDownloadingUpdate).toBe(false)
    expect(store.isUpdateReady).toBe(true)
    expect(store.updatePhase).toBe('ready')
    expect(store.updateProgress?.stage).toBe('ready')

    // Defer closes the dialog while preserving the staged ready update.
    store.deferInstall()
    expect(store.updateDialogOpen).toBe(false)
    expect(store.isUpdateReady).toBe(true)

    store.showUpdateDialog()
    expect(store.updateDialogOpen).toBe(true)
    expect(store.updatePhase).toBe('ready')

    const installPromise = store.installUpdate()
    expect(store.isInstallingUpdate).toBe(true)
    expect(store.updatePhase).toBe('installing')

    rejectInstall(new Error('已取消管理员授权，更新未安装'))
    await installPromise

    expect(store.isInstallingUpdate).toBe(false)
    expect(store.isUpdateReady).toBe(true)
    expect(store.updatePhase).toBe('ready')
    expect(store.updateDialogOpen).toBe(true)
    expect(noticeStore.items).toHaveLength(1)
    expect(noticeStore.items[0]).toMatchObject({
      tone: 'warning',
      title: '已取消管理员授权',
      message: '更新未安装',
    })
  })

  it('cancels an in-flight download silently, ignores trailing events, and resets ready state when staged verification fails', async () => {
    let rejectDownload!: (reason?: unknown) => void
    const progressSub: { emit?: (progress: UpdateProgress) => void } = {}
    vi.spyOn(systemApi, 'subscribeUpdateProgress').mockImplementation((listener) => {
      progressSub.emit = listener
      return () => {
        progressSub.emit = undefined
      }
    })
    vi.spyOn(systemApi, 'downloadUpdate').mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectDownload = reject
        }),
    )
    const cancelSpy = vi.spyOn(systemApi, 'cancelUpdate').mockResolvedValue(true)

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
      assetSize: 1024,
      assetSha256: 'abc',
    }
    store.updateDialogOpen = true

    const dlPromise = store.downloadUpdate()
    await store.cancelUpdate()
    expect(cancelSpy).toHaveBeenCalledTimes(1)
    expect(store.isCancelingUpdate).toBe(true)

    rejectDownload(new Error('已取消下载更新'))
    await dlPromise

    // Stale async event arriving after cancellation must not pollute idle state.
    progressSub.emit?.({
      stage: 'downloading',
      downloadedBytes: 512,
      totalBytes: 1024,
      percent: 50,
      message: '正在下载更新…',
    })

    expect(store.isDownloadingUpdate).toBe(false)
    expect(store.isCancelingUpdate).toBe(false)
    expect(store.updatePhase).toBe('available')
    expect(store.updateProgress).toBeNull()
    expect(noticeStore.items).toHaveLength(0)

    // Tampered/missing staged package during install resets back to available.
    vi.spyOn(systemApi, 'downloadUpdate').mockResolvedValue()
    vi.spyOn(systemApi, 'installUpdate').mockRejectedValue(
      new Error('更新安装包 SHA-256 校验失败，已中止安装'),
    )
    await store.downloadUpdate()
    expect(store.isUpdateReady).toBe(true)

    await store.installUpdate()
    expect(store.isUpdateReady).toBe(false)
    expect(store.updatePhase).toBe('available')
    expect(store.updateProgress).toBeNull()
    expect(noticeStore.items).toHaveLength(1)
    expect(noticeStore.items[0]?.tone).toBe('danger')
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
