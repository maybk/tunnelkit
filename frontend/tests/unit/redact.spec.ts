import { describe, expect, it } from 'vite-plus/test'
import { redactSensitiveText } from '@/utils/redact'

describe('credential redaction', () => {
  it.each([
    'SVPNCOOKIE=audit-cookie',
    'credential="audit credential with spaces" host=vpn.example.com',
    "otp: 'audit-otp'",
    'Authorization: Bearer audit-token',
    'Proxy-Authorization: Basic audit-token',
    'Cookie: first=audit-one; second=audit-two',
    '{"password":"audit secret","host":"vpn.example.com"}',
    'password="audit unterminated',
    'password="audit first\naudit second"\nhost=vpn.example.com',
    '{"token":["audit-one","audit-two"]}',
    '{"secret":{"nested":"audit-value"}}',
    '{"password":\n"audit-multiline-json"}',
  ])('removes synthetic credentials from %s', (input) => {
    expect(redactSensitiveText(input)).not.toContain('audit')
  })

  it('keeps diagnostic fields and redacts every secret on a line', () => {
    const output = redactSensitiveText('password="audit one" token=audit-two host=vpn.example.com')
    expect(output).not.toContain('audit')
    expect(output).toContain('host=vpn.example.com')
    expect(redactSensitiveText(output)).toBe(output)
  })
})
