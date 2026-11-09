import { Browser } from '@wailsio/runtime'
import { describe, expect, it, vi } from 'vite-plus/test'
import { systemApi } from '@/api/system'
import { vpnApi } from '@/api/vpn'

describe('external browser link boundaries', () => {
  it('allows the official repository and release paths only for system links', async () => {
    vi.mocked(Browser.OpenURL).mockClear()
    await systemApi.openExternalUrl('https://github.com/maybk/tunnelkit')
    await systemApi.openExternalUrl('https://github.com/maybk/tunnelkit/releases/tag/v1.0.0')
    for (const url of [
      'https://github.com/maybk/tunnelkit/releases-evil',
      'https://user:pass@github.com/maybk/tunnelkit/releases',
      'https://github.com:8443/maybk/tunnelkit/releases',
      'https://github.com/other/repo',
    ]) {
      await expect(systemApi.openExternalUrl(url)).rejects.toThrow()
    }
    expect(Browser.OpenURL).toHaveBeenCalledTimes(2)
  })

  it('allows HTTPS gateway authentication without the GitHub release restriction', async () => {
    vi.mocked(Browser.OpenURL).mockClear()
    await vpnApi.openAuthenticationUrl('https://vpn.example.com:10443/remote/saml/start')
    for (const url of [
      'javascript:alert(1)',
      'file:///tmp/auth',
      'http://vpn.example.com',
      'https://user:pass@vpn.example.com',
    ]) {
      await expect(vpnApi.openAuthenticationUrl(url)).rejects.toThrow()
    }
    expect(Browser.OpenURL).toHaveBeenCalledOnce()
  })
})
