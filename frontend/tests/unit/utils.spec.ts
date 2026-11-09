import { describe, expect, it } from 'vite-plus/test'
import { formatBytes, formatCount, formatRtt, splitMultilineItems } from '@/utils/format'
import {
  formatEndpoint,
  isValidIpv4,
  isValidIpv4Cidr,
  normalizeHostInput,
  parseGatewayEndpoint,
} from '@/utils/network'
import { redactSensitiveText } from '@/utils/redact'
import { formatDurationSeconds, formatElapsedSince } from '@/utils/time'

describe('utils', () => {
  it('formats bytes, counts, RTT, and multiline items accurately', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(20 * 1024 * 1024)).toBe('20.0 MB')
    expect(formatCount(1234)).toBe('1,234')
    expect(formatRtt(28.4)).toBe('28 ms')
    expect(splitMultilineItems('10.0.0.0/8\n192.168.1.0/24, 10.0.0.0/8')).toEqual([
      '10.0.0.0/8',
      '192.168.1.0/24',
    ])
  })

  it('normalizes host, endpoint with embedded port, IPv4, and CIDR', () => {
    expect(normalizeHostInput('https://vpn.example.com:8443/remote/login')).toBe(
      'vpn.example.com:8443',
    )
    expect(parseGatewayEndpoint('https://vpn.example.com:8443/path')).toEqual({
      host: 'vpn.example.com',
      port: 8443,
    })
    expect(parseGatewayEndpoint('[2001:db8::1]:10443')).toEqual({
      host: '2001:db8::1',
      port: 10443,
    })
    expect(isValidIpv4('192.168.1.1')).toBe(true)
    expect(isValidIpv4('999.1.1.1')).toBe(false)
    expect(isValidIpv4Cidr('10.0.0.0/8')).toBe(true)
    expect(isValidIpv4Cidr('10.0.0.0/33')).toBe(false)
    expect(formatEndpoint('vpn.example.com', 8443)).toBe('vpn.example.com:8443')
  })

  it('redacts sensitive tokens, cookies, and passwords in log text', () => {
    const raw = 'Authorization=BearerSecret token=abc123def password: my-secret otp=654321'
    const redacted = redactSensitiveText(raw)
    expect(redacted).not.toContain('BearerSecret')
    expect(redacted).not.toContain('abc123def')
    expect(redacted).not.toContain('my-secret')
    expect(redacted).not.toContain('654321')
    expect(redacted).toContain('[REDACTED]')
  })

  it('formats durations and elapsed times consistently', () => {
    expect(formatDurationSeconds(65)).toBe('00:01:05')
    expect(formatDurationSeconds(3661)).toBe('01:01:01')
    const now = Date.parse('2026-04-01T12:05:00Z')
    expect(formatElapsedSince('2026-04-01T12:00:00Z', now)).toBe('00:05:00')
  })
})
