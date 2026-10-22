import type { Profile, ProfileInput } from '@/api/profile'

export const DEFAULT_VPN_PORT = 443

export const CONFIG_TEXT_PLACEHOLDER = `host = vpn.example.com
port = 443
username = your_account
password = your_password
realm = corp
routes = gateway
dns = split
dns-domain = corp.example.com
reconnect = 1`

export interface ProfileFormState {
  name: string
  host: string
  port: number
  username: string
  password: string
  realm: string
  trustedCert: string
  auth: string
  routes: string
  extraRoutesText: string
  dns: string
  dnsDomainsText: string
  reconnect: boolean
  userAgent: string
  legacyTls: boolean
  clientCert: string
  clientKey: string
}

export const splitList = (raw: string): string[] =>
  raw
    .split(/[\n,;]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)

export const createProfileFormState = (): ProfileFormState => ({
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
})

export const formStateToInput = (form: ProfileFormState, id = ''): ProfileInput => ({
  id,
  name: form.name.trim(),
  host: form.host.trim(),
  port: Number(form.port) || DEFAULT_VPN_PORT,
  username: form.username.trim(),
  password: form.password,
  realm: form.realm.trim(),
  trustedCert: form.trustedCert.trim(),
  auth: form.auth,
  routes: form.routes,
  extraRoutes: splitList(form.extraRoutesText),
  dns: form.dns,
  dnsDomains: splitList(form.dnsDomainsText),
  reconnect: form.reconnect,
  userAgent: form.userAgent.trim(),
  legacyTls: form.legacyTls,
  clientCert: form.clientCert.trim(),
  clientKey: form.clientKey.trim(),
})

export const profileToBaselineInput = (src?: Profile | null): ProfileInput => ({
  id: src?.id ?? '',
  name: (src?.name ?? '').trim(),
  host: (src?.host ?? '').trim(),
  port: Number(src?.port) || DEFAULT_VPN_PORT,
  username: (src?.username ?? '').trim(),
  password: '',
  realm: (src?.realm ?? '').trim(),
  trustedCert: (src?.trustedCert ?? '').trim(),
  auth: src?.auth || 'password',
  routes: src?.routes || 'gateway',
  extraRoutes: splitList((src?.extraRoutes ?? []).join('\n')),
  dns: src?.dns || 'split',
  dnsDomains: splitList((src?.dnsDomains ?? []).join(', ')),
  reconnect: src ? Boolean(src.reconnect) : true,
  userAgent: (src?.userAgent ?? '').trim(),
  legacyTls: Boolean(src?.legacyTls),
  clientCert: (src?.clientCert ?? '').trim(),
  clientKey: (src?.clientKey ?? '').trim(),
})

export const applyInputToFormState = (
  form: ProfileFormState,
  inp: ProfileInput,
  options?: { includeName?: boolean },
): void => {
  if (options?.includeName) {
    form.name = inp.name ?? ''
  }
  form.host = inp.host ?? ''
  form.port = inp.port || DEFAULT_VPN_PORT
  form.username = inp.username ?? ''
  form.password = inp.password ?? ''
  form.realm = inp.realm ?? ''
  form.trustedCert = inp.trustedCert ?? ''
  form.auth = inp.auth || 'password'
  form.routes = inp.routes || 'gateway'
  form.extraRoutesText = (inp.extraRoutes ?? []).join('\n')
  form.dns = inp.dns || 'split'
  form.dnsDomainsText = (inp.dnsDomains ?? []).join(', ')
  form.reconnect = Boolean(inp.reconnect)
  form.userAgent = inp.userAgent ?? ''
  form.legacyTls = Boolean(inp.legacyTls)
  form.clientCert = inp.clientCert ?? ''
  form.clientKey = inp.clientKey ?? ''
}

export const hasAdvancedFormValues = (form: ProfileFormState): boolean =>
  Boolean(
    form.realm.trim() ||
    form.trustedCert.trim() ||
    form.auth === 'saml' ||
    form.userAgent.trim() ||
    form.legacyTls ||
    form.clientCert.trim() ||
    form.clientKey.trim(),
  )
