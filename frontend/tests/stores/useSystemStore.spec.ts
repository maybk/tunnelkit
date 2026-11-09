import { describe, expect, it, vi } from 'vite-plus/test'
import { systemApi } from '@/api/system'
import { useSystemStore } from '@/stores/useSystemStore'

describe('useSystemStore', () => {
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

    store.aboutDialogOpen = true
    const info = await store.checkUpdate({ silent: false })
    expect(info?.hasUpdate).toBe(true)
    expect(store.hasUpdate).toBe(true)
    expect(store.aboutDialogOpen).toBe(false)
    expect(store.updateDialogOpen).toBe(true)
  })
})
