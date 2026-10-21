import { describe, expect, it } from 'vitest'
import { profileStatus } from '@/lib/status'

describe('profileStatus', () => {
  it('maps the target profile through every lifecycle state', () => {
    expect(profileStatus('p1', { status: 'connected', profileId: 'p1' })).toMatchObject({
      isTarget: true,
      status: 'connected',
      tone: 'ok',
      otherActive: false,
    })
    expect(profileStatus('p1', { status: 'connecting', profileId: 'p1' }).tone).toBe('busy')
    expect(profileStatus('p1', { status: 'disconnecting', profileId: 'p1' }).tone).toBe('busy')
    expect(
      profileStatus('p1', { status: 'disconnected', profileId: 'p1', error: { message: 'x' } }),
    ).toMatchObject({ tone: 'danger', error: { message: 'x' } })
  })

  it('treats other profiles as disconnected and flags the active one', () => {
    const view = profileStatus('p2', {
      status: 'connected',
      profileId: 'p1',
      error: { message: 'belongs to p1' },
    })
    expect(view).toMatchObject({
      isTarget: false,
      status: 'disconnected',
      tone: 'idle',
      error: null,
      otherActive: true,
    })
  })
})
