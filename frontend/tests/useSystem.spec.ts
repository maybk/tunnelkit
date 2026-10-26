import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { UpdateProgress } from '@/api/system'

const mockInfo = vi.fn()
const mockCheckUpdate = vi.fn()
const mockApplyUpdate = vi.fn()
const mockOpenURL = vi.fn()
const mockOpenRepo = vi.fn()
let progressListener: ((progress: UpdateProgress) => void) | null = null

vi.mock('@/api/system', () => ({
  getSystemInfo: () => mockInfo(),
  checkSystemUpdate: () => mockCheckUpdate(),
  applySystemUpdate: () => mockApplyUpdate(),
  subscribeUpdateProgress: (cb: (progress: UpdateProgress) => void) => {
    progressListener = cb
    return () => {
      progressListener = null
    }
  },
  openReleaseUrl: (url: string) => mockOpenURL(url),
  openGitHubRepo: () => mockOpenRepo(),
}))

describe('useSystem update workflow', () => {
  beforeEach(() => {
    vi.resetModules()
    mockInfo.mockReset()
    mockCheckUpdate.mockReset()
    mockApplyUpdate.mockReset()
    mockOpenURL.mockReset()
    mockOpenRepo.mockReset()
    progressListener = null
    globalThis.localStorage?.clear()
  })

  it('prompts on silent check unless version was dismissed, and always prompts on manual check', async () => {
    const { useSystem } = await import('@/stores/useSystem')
    const sys = useSystem()

    mockCheckUpdate.mockResolvedValue({
      currentVersion: '0.1.0',
      latestVersion: '0.1.1',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.1.1',
    })

    await sys.checkUpdate({ silent: true })
    expect(sys.hasUpdate.value).toBe(true)
    expect(sys.updateDialogOpen.value).toBe(true)

    sys.dismissUpdate('0.1.1')
    expect(sys.updateDialogOpen.value).toBe(false)

    // 手动检查即使已忽略也应弹出
    await sys.checkUpdate({ silent: false })
    expect(sys.updateDialogOpen.value).toBe(true)

    await sys.openUpdateRelease()
    expect(mockOpenURL).toHaveBeenCalledWith(
      'https://github.com/maybk/tunnelkit/releases/tag/v0.1.1',
    )
    expect(sys.updateDialogOpen.value).toBe(false)

    await sys.openRepository()
    expect(mockOpenRepo).toHaveBeenCalledTimes(1)
  })

  it('sets up-to-date feedback when manual check finds no newer version', async () => {
    const { useSystem } = await import('@/stores/useSystem')
    const sys = useSystem()

    mockCheckUpdate.mockResolvedValue({
      currentVersion: '0.1.1',
      latestVersion: '0.1.1',
      hasUpdate: false,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.1.1',
    })

    await sys.checkUpdate({ silent: false })
    expect(sys.hasUpdate.value).toBe(false)
    expect(sys.updateDialogOpen.value).toBe(false)
    expect(sys.updateFeedback.value).toBe('当前已是最新版本')

    // 手动检查执行过后，延后触发的静默自动检查不再重复发起请求
    await sys.checkUpdate({ silent: true })
    expect(mockCheckUpdate).toHaveBeenCalledTimes(1)
  })

  it('tracks progress events during applyUpdate and resets state on completion', async () => {
    const { useSystem } = await import('@/stores/useSystem')
    const sys = useSystem()

    mockCheckUpdate.mockResolvedValue({
      currentVersion: '0.2.0',
      latestVersion: '0.2.1',
      hasUpdate: true,
      releaseUrl: 'https://github.com/maybk/tunnelkit/releases/tag/v0.2.1',
      assetSize: 1000,
      canSelfUpdate: true,
    })
    await sys.checkUpdate({ silent: false })

    mockApplyUpdate.mockImplementation(async () => {
      expect(sys.applyingUpdate.value).toBe(true)
      progressListener?.({
        stage: 'downloading',
        downloadedBytes: 600,
        totalBytes: 1000,
        percent: 60,
        message: '正在下载更新安装包 (60%)…',
      })
      expect(sys.updateProgress.value?.percent).toBe(60)
    })

    await sys.applyUpdate()
    expect(mockApplyUpdate).toHaveBeenCalledTimes(1)
    expect(sys.applyingUpdate.value).toBe(false)

    mockApplyUpdate.mockRejectedValueOnce(new Error('SHA-256 mismatch'))
    await expect(sys.applyUpdate()).rejects.toThrow('SHA-256 mismatch')
    expect(sys.applyingUpdate.value).toBe(false)
    expect(sys.updateProgress.value).toBeNull()
    sys.stopWatching()
  })
})
