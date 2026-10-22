import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockInfo = vi.fn()
const mockCheckUpdate = vi.fn()
const mockOpenURL = vi.fn()
const mockOpenRepo = vi.fn()

vi.mock('@/api/system', () => ({
  getSystemInfo: () => mockInfo(),
  checkSystemUpdate: () => mockCheckUpdate(),
  openReleaseUrl: (url: string) => mockOpenURL(url),
  openGitHubRepo: () => mockOpenRepo(),
}))

describe('useSystem update workflow', () => {
  beforeEach(() => {
    vi.resetModules()
    mockInfo.mockReset()
    mockCheckUpdate.mockReset()
    mockOpenURL.mockReset()
    mockOpenRepo.mockReset()
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
})
