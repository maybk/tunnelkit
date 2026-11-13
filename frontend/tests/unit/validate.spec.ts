import { describe, expect, it } from 'vite-plus/test'
import { toProfileDraft } from '@/domain/profile'
import { formatErrorMessage, isElevationCancelledError } from '@/presentation/error'
import { describeVpnPhase, getStatusMeta, getViewingStatusMeta } from '@/presentation/vpn'
import { validateProfileDraft } from '@/validate/profile'
import { parseCachedAppearance } from '@/validate/storage'
import { isTrustedReleaseUrl } from '@/validate/update'
import { validateOtpCode } from '@/validate/vpn'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

describe('validate and presentation layers', () => {
  it('validates profile drafts and OTP challenge codes', () => {
    const validDraft = toProfileDraft(createTestProfile())
    const okResult = validateProfileDraft(validDraft)
    expect(okResult.valid).toBe(true)

    const missingPasswordDraft = {
      ...validDraft,
      hasPassword: false,
      password: '',
    }
    const missingPwdResult = validateProfileDraft(missingPasswordDraft)
    expect(missingPwdResult.valid).toBe(false)
    expect(missingPwdResult.errors.password).toBeTruthy()

    const invalidDraft = {
      ...validDraft,
      name: '',
      host: 'bad host',
      trustedCert: 'not-a-hex-digest',
    }
    const badResult = validateProfileDraft(invalidDraft)
    expect(badResult.valid).toBe(false)
    expect(badResult.errors.name).toBeTruthy()
    expect(badResult.errors.host).toBeTruthy()
    expect(badResult.errors.trustedCert).toBeTruthy()

    expect(validateOtpCode('   ').valid).toBe(false)
    expect(validateOtpCode(' 123456 ').valid).toBe(true)
    expect(validateOtpCode(' 123456 ').code).toBe('123456')
  })

  it.each([0, -1, 65536, 1.5, Number.NaN])('rejects invalid explicit port %s', (port) => {
    const draft = toProfileDraft(createTestProfile({ port: 10443 }))
    draft.port = port
    const result = validateProfileDraft(draft)
    expect(result.valid).toBe(false)
    expect(result.errors.port).toBeTruthy()
  })

  it('validates cached appearance JSON and trusted release URLs', () => {
    expect(parseCachedAppearance('{"mode":"dark"}').mode).toBe('dark')
    expect(parseCachedAppearance('invalid-json').mode).toBe('system')

    expect(isTrustedReleaseUrl('https://github.com/maybk/tunnelkit/releases')).toBe(true)
    expect(isTrustedReleaseUrl('http://github.com/maybk/tunnelkit/releases')).toBe(false)
    expect(isTrustedReleaseUrl('https://evil.example.com/releases')).toBe(false)
  })

  it('resolves semantic VPN status meta, phase description, and error messages', () => {
    const connected = createTestVpnState({
      status: 'connected',
      phase: 'connected',
      profileId: 'profile-1',
      profileName: '总部网关',
      info: {
        interface: 'utun8',
        localIp: '10.212.134.10',
        peerIp: '10.212.134.1',
        mtu: 1400,
        routeCount: 2,
        routes: ['10.0.0.0/8'],
        dnsMode: 'split',
        dnsServers: ['10.0.0.53'],
        dnsDomains: ['corp.local'],
        splitDnsNeedsDomains: false,
        gatewayHost: 'vpn.example.com',
        gatewayIp: '203.0.113.10',
        fortiOs: 'v7.4',
      },
    })
    expect(getStatusMeta('connected').tone).toBe('success')
    expect(getViewingStatusMeta(connected, 'profile-1').tone).toBe('success')
    expect(describeVpnPhase(connected, createTestProfile())).toContain('10.212.134.10')
    expect(describeVpnPhase(connected, createTestProfile({ id: 'profile-2' }))).toBe(
      '当前「总部网关」已连接',
    )
    expect(describeVpnPhase(createTestVpnState(), createTestProfile())).toBe(
      '目标网关 vpn.example.com:443',
    )

    expect(formatErrorMessage(new Error('连接超时'))).toBe('连接超时')
    expect(formatErrorMessage({ error: '认证被拒绝' })).toBe('认证被拒绝')
    expect(formatErrorMessage(null, '默认错误')).toBe('默认错误')
    expect(isElevationCancelledError(new Error('已取消管理员授权，更新未安装'))).toBe(true)
    expect(isElevationCancelledError({ message: 'Authorization cancelled by user' })).toBe(true)
    expect(isElevationCancelledError(new Error('下载失败'))).toBe(false)
  })
})
