import { describe, expect, it } from 'vitest'
import { errorMessage, formatBytes, formatClockTime, formatDurationSince } from '@/lib/format'

describe('format helpers', () => {
  it('formats byte counts across units', () => {
    expect(formatBytes(undefined)).toBe('0 B')
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(2048)).toBe('2.0 KB')
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB')
  })

  it('formats elapsed duration since ISO timestamp', () => {
    const start = '2026-10-21T08:00:00Z'
    const baseMs = Date.parse(start)

    expect(formatDurationSince(undefined, baseMs)).toBe('')
    expect(formatDurationSince('invalid-date', baseMs)).toBe('')
    expect(formatDurationSince(start, baseMs + 45_000)).toBe('45 秒')
    expect(formatDurationSince(start, baseMs + 12 * 60_000)).toBe('12 分钟')
    expect(formatDurationSince(start, baseMs + 120 * 60_000)).toBe('2 小时')
    expect(formatDurationSince(start, baseMs + 135 * 60_000)).toBe('2 小时 15 分钟')
  })

  it('formats clock time and normalizes errors', () => {
    expect(formatClockTime(undefined)).toBe('—')
    expect(formatClockTime('not-a-time')).toBe('—')
    expect(formatClockTime('2026-10-21T08:05:09Z')).toMatch(/^\d{2}:\d{2}:\d{2}$/)

    expect(errorMessage(new Error('连接超时'))).toBe('连接超时')
    expect(errorMessage('认证被拒绝')).toBe('认证被拒绝')
    expect(errorMessage(null)).toBe('操作失败，请重试')
  })
})
