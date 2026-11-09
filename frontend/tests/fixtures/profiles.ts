import type { Profile, ProfileInput } from '@/types/profile'
import type { VpnState } from '@/types/vpn'

export function createTestProfile(overrides: Partial<Profile> = {}): Profile {
  return {
    id: 'profile-1',
    name: '总部网关',
    host: 'vpn.example.com',
    port: 443,
    username: 'alice',
    realm: 'corp',
    trustedCert: '',
    auth: 'password',
    routes: 'gateway',
    extraRoutes: [],
    dns: 'split',
    dnsDomains: [],
    reconnect: true,
    userAgent: '',
    legacyTls: false,
    clientCert: '',
    clientKey: '',
    hasPassword: true,
    updatedAt: '2026-04-01T12:00:00Z',
    ...overrides,
  }
}

export function createTestProfileInput(overrides: Partial<ProfileInput> = {}): ProfileInput {
  return {
    id: 'profile-1',
    name: '总部网关',
    host: 'vpn.example.com',
    port: 443,
    username: 'alice',
    password: '',
    realm: 'corp',
    trustedCert: '',
    auth: 'password',
    routes: 'gateway',
    extraRoutes: [],
    dns: 'split',
    dnsDomains: [],
    reconnect: true,
    userAgent: '',
    legacyTls: false,
    clientCert: '',
    clientKey: '',
    ...overrides,
  }
}

export function createTestVpnState(overrides: Partial<VpnState> = {}): VpnState {
  return {
    status: 'disconnected',
    phase: 'disconnected',
    profileId: '',
    profileName: '',
    connectedAt: '',
    info: null,
    stats: null,
    prompt: null,
    error: null,
    ...overrides,
  }
}
