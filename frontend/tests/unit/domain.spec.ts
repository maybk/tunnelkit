import { describe, expect, it } from 'vite-plus/test'
import { filterLogLines, formatLogsForExport, parseLogContent } from '@/domain/log'
import {
  createDefaultProfileDraft,
  isProfileDraftEqual,
  toProfileDraft,
  toProfileInput,
} from '@/domain/profile'
import {
  isActiveStatus,
  isProfileActive,
  isTransitionStatus,
  needsSwitchConfirm,
} from '@/domain/vpn'
import { createTestProfile, createTestVpnState } from '../fixtures/profiles'

describe('domain logic', () => {
  it('evaluates VPN status transitions, active profile, and switch confirmation', () => {
    const disconnected = createTestVpnState({ status: 'disconnected', phase: 'disconnected' })
    expect(isActiveStatus(disconnected.status)).toBe(false)
    expect(isTransitionStatus(disconnected.status)).toBe(false)

    const connecting = createTestVpnState({
      status: 'connecting',
      phase: 'authenticating',
      profileId: 'p1',
    })
    expect(isTransitionStatus(connecting.status)).toBe(true)
    expect(isProfileActive(connecting, 'p1')).toBe(true)
    expect(needsSwitchConfirm(connecting, 'p2')).toBe(true)
  })

  it('converts profile to draft and detects unsaved modifications', () => {
    const profile = createTestProfile({
      extraRoutes: ['10.0.0.0/8', '172.16.0.0/12'],
      dnsDomains: ['corp.local', 'internal.example.com'],
    })
    const draft = toProfileDraft(profile)
    expect(draft.extraRoutesText).not.toContain('\n')
    expect(toProfileInput(draft).extraRoutes).toEqual(profile.extraRoutes)
    expect(toProfileInput(draft).dnsDomains).toEqual(profile.dnsDomains)
    expect(isProfileDraftEqual(draft, toProfileDraft(profile))).toBe(true)

    const modified = {
      ...draft,
      host: 'https://gw.corp.example.com/login',
      port: 10443,
    }
    expect(isProfileDraftEqual(modified, draft)).toBe(false)

    const input = toProfileInput(modified)
    expect(input.host).toBe('gw.corp.example.com')
    expect(input.port).toBe(10443)
    expect(createDefaultProfileDraft().port).toBe(443)
  })

  it('parses, filters, and exports log lines with redaction', () => {
    const rawContent = [
      '2026-04-01T12:00:00Z [INFO] connected token=secret123',
      '2026-04-01T12:00:02Z [ERROR] TLS handshake failed',
    ].join('\n')
    const lines = parseLogContent(rawContent)
    expect(lines).toHaveLength(2)
    expect(lines[0]?.level).toBe('INFO')
    expect(lines[1]?.level).toBe('ERROR')

    const onlyErrors = filterLogLines(lines, 'ERROR', 'tls')
    expect(onlyErrors).toHaveLength(1)

    const exported = formatLogsForExport(lines)
    expect(exported).toContain('token=[REDACTED]')
    expect(exported).toContain('TLS handshake failed')
  })
})
