import { DEFAULT_VPN_PORT } from '@/constants/profile'
import type { Profile, ProfileDraft, ProfileInput } from '@/types/profile'
import { splitMultilineItems } from '@/utils/format'
import { normalizeHostInput } from '@/utils/network'

export function createDefaultProfileDraft(): ProfileDraft {
  return {
    id: '',
    name: '',
    host: '',
    port: DEFAULT_VPN_PORT,
    username: '',
    password: '',
    realm: '',
    trustedCert: '',
    auth: 'password',
    routes: 'gateway',
    extraRoutesText: '',
    dns: 'split',
    dnsDomainsText: '',
    reconnect: true,
    userAgent: '',
    legacyTls: false,
    clientCert: '',
    clientKey: '',
    hasPassword: false,
  }
}

export function toProfileDraft(profile?: Profile | null): ProfileDraft {
  if (!profile) return createDefaultProfileDraft()
  return {
    id: profile.id,
    name: profile.name,
    host: profile.host,
    port: profile.port || DEFAULT_VPN_PORT,
    username: profile.username,
    password: '',
    realm: profile.realm ?? '',
    trustedCert: profile.trustedCert ?? '',
    auth: profile.auth || 'password',
    routes: profile.routes || 'gateway',
    extraRoutesText: (profile.extraRoutes ?? []).join(', '),
    dns: profile.dns || 'split',
    dnsDomainsText: (profile.dnsDomains ?? []).join(', '),
    reconnect: profile.reconnect ?? true,
    userAgent: profile.userAgent ?? '',
    legacyTls: Boolean(profile.legacyTls),
    clientCert: profile.clientCert ?? '',
    clientKey: profile.clientKey ?? '',
    hasPassword: Boolean(profile.hasPassword),
  }
}

export function toProfileInput(draft: ProfileDraft): ProfileInput {
  return {
    id: draft.id.trim(),
    name: draft.name.trim(),
    host: normalizeHostInput(draft.host),
    port: Number(draft.port),
    username: draft.username.trim(),
    password: draft.password,
    realm: draft.realm.trim(),
    trustedCert: draft.trustedCert.trim(),
    auth: draft.auth,
    routes: draft.routes,
    extraRoutes: splitMultilineItems(draft.extraRoutesText),
    dns: draft.dns,
    dnsDomains: splitMultilineItems(draft.dnsDomainsText),
    reconnect: Boolean(draft.reconnect),
    userAgent: draft.userAgent.trim(),
    legacyTls: Boolean(draft.legacyTls),
    clientCert: draft.clientCert.trim(),
    clientKey: draft.clientKey.trim(),
  }
}

function areStringArraysEqual(a: readonly string[], b: readonly string[]): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

export function isProfileDraftEqual(a: ProfileDraft, b: ProfileDraft): boolean {
  const inputA = toProfileInput(a)
  const inputB = toProfileInput(b)
  return (
    inputA.id === inputB.id &&
    inputA.name === inputB.name &&
    inputA.host === inputB.host &&
    inputA.port === inputB.port &&
    inputA.username === inputB.username &&
    inputA.password === inputB.password &&
    inputA.realm === inputB.realm &&
    inputA.trustedCert === inputB.trustedCert &&
    inputA.auth === inputB.auth &&
    inputA.routes === inputB.routes &&
    areStringArraysEqual(inputA.extraRoutes, inputB.extraRoutes) &&
    inputA.dns === inputB.dns &&
    areStringArraysEqual(inputA.dnsDomains, inputB.dnsDomains) &&
    inputA.reconnect === inputB.reconnect &&
    inputA.userAgent === inputB.userAgent &&
    inputA.legacyTls === inputB.legacyTls &&
    inputA.clientCert === inputB.clientCert &&
    inputA.clientKey === inputB.clientKey &&
    a.hasPassword === b.hasPassword
  )
}
